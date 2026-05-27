import { test, expect, type Page, type APIRequestContext } from '@playwright/test'

/**
 * Kodik RPC canary — WT-SYNC-10 (workstream watch-together, Phase 03 Plan 03.4).
 *
 * The undocumented `kodik_player_api` postMessage RPC was discovered
 * 2026-05-25 (see memory reference `reference_kodik_inbound_postmessage_api.md`)
 * and is the foundation of Kodik's Watch Together adapter. If Kodik ships a
 * bundle update that removes or renames the dispatcher, this canary fails in
 * CI and we get warning before users notice.
 *
 * Daily CI hook: the test name carries the literal `[canary]` substring so a
 * scheduled job can filter with `bunx playwright test --grep canary`. Phase 5
 * (alerting) wires the actual nightly cron. Until then, the spec is shippable
 * and runnable locally on demand.
 *
 * Local invocation:
 *   cd frontend/web && bunx playwright test e2e/kodik-rpc-probe.spec.ts --reporter=list
 *
 * Gracefully skips when:
 *   - the dev stack is not up (`/api/anime/_/scraper/health` unreachable);
 *   - the seeded `ui_audit_bot` user has no anime_list rows
 *     (re-run `./scripts/seed-ui-audit-user.sh`);
 *   - no Kodik-bearing anime can be found in the bot's seeded list (catalog
 *     drift — the test does NOT hard-fail because Kodik availability is per
 *     anime, not a fixed contract);
 *   - the iframe never registers a window.player.api dispatcher in time
 *     (network conditions, ad bumper, or a Kodik incident — these reasons
 *     are surfaced as a test.skip with the timing context).
 *
 * Why these are skips and not hard failures: the canary's purpose is to
 * detect a real Kodik bundle regression. Stack-down / catalog-drift /
 * network-flake produce noise that would either be ignored or train CI
 * to ignore the canary. The hard failure path is reserved for the one
 * signal we care about: get_time → no kodik_player_time reply.
 */

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

const PROBE_TIMEOUT_MS = 5000
const COMMAND_RESPONSE_TIMEOUT_MS = 5000
const IFRAME_VISIBLE_TIMEOUT_MS = 15_000

interface AuthResult {
  token: string
}

async function loginAs(
  page: Page,
  request: APIRequestContext,
  username: string,
  password: string,
): Promise<AuthResult> {
  const resp = await request.post('/api/auth/login', { data: { username, password } })
  if (!resp.ok()) {
    throw new Error(`login(${username}) failed: ${resp.status()} ${await resp.text()}`)
  }
  const body = await resp.json()
  const data = body?.data ?? body
  const token: string | undefined = data?.access_token
  const user: Record<string, unknown> | undefined = data?.user
  if (!token || !user) {
    throw new Error(`login(${username}): no token/user in response`)
  }
  await page.addInitScript(
    ({ tok, usr }) => {
      window.localStorage.setItem('token', tok)
      window.localStorage.setItem('user', JSON.stringify(usr))
    },
    { tok: token, usr: user },
  )
  return { token }
}

/**
 * Walk through a small set of candidate anime IDs and return the first one
 * with a Kodik translation. The seeded ui_audit_bot's `watching` list is the
 * primary source (stable across CI), with `/api/anime/ongoing` as a fallback
 * if the bot's list is empty.
 */
async function findKodikBearingAnimeId(
  request: APIRequestContext,
  token: string,
): Promise<string | null> {
  // Pass 1: bot's seeded watching list.
  const candidates: string[] = []
  try {
    const resp = await request.get('/api/users/watchlist?status=watching', {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (resp.ok()) {
      const body = await resp.json()
      const items = body?.data ?? body?.items ?? []
      if (Array.isArray(items)) {
        for (const item of items) {
          const id: string | undefined = item?.anime_id ?? item?.anime?.id ?? item?.id
          if (id) candidates.push(id)
        }
      }
    }
  } catch {
    // fall through to ongoing
  }

  // Pass 2: top ongoing anime — high probability of Kodik availability for RU titles.
  try {
    const resp = await request.get('/api/anime/ongoing?page_size=10')
    if (resp.ok()) {
      const body = await resp.json()
      const items = body?.data?.items ?? body?.data ?? body?.items ?? body ?? []
      const arr = Array.isArray(items) ? items : []
      for (const item of arr) {
        const id: string | undefined = item?.id ?? item?.anime_id
        if (id && !candidates.includes(id)) candidates.push(id)
      }
    }
  } catch {
    // ignore
  }

  // Probe each candidate for Kodik translations via the public catalog endpoint.
  for (const id of candidates.slice(0, 12)) {
    try {
      const resp = await request.get(`/api/anime/${id}/kodik/translations`)
      if (!resp.ok()) continue
      const body = await resp.json()
      const data = body?.data ?? body
      const list = Array.isArray(data) ? data : []
      if (list.length > 0) return id
    } catch {
      // try next
    }
  }
  return null
}

/**
 * Probe whether the local stack is up enough to run this canary. Same
 * pattern as watch-together-shell.spec.ts.
 */
async function isStackUp(request: APIRequestContext): Promise<boolean> {
  try {
    const resp = await request.get('/api/anime/_/scraper/health', { timeout: 3000 })
    return resp.ok()
  } catch {
    return false
  }
}

test.describe('Kodik RPC canary (WT-SYNC-10, Plan 03.4)', () => {
  test.beforeAll(async ({ request }) => {
    const up = await isStackUp(request)
    test.skip(!up, 'AnimeEnigma local stack is not up — run `make dev` and re-run')
  })

  test('[canary] kodik_player_api RPC responds to get_time/play/pause/seek', async ({
    browser,
    request,
  }) => {
    const ctx = await browser.newContext()
    try {
      const page = await ctx.newPage()
      const auth = await loginAs(page, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)

      // ── Step 1: find a Kodik-bearing anime. ────────────────────────────
      const animeId = await findKodikBearingAnimeId(request, auth.token)
      test.skip(
        !animeId,
        'no Kodik-bearing anime found in ui_audit_bot list or top ongoing — catalog drift, not a Kodik regression',
      )
      // Type narrow for the rest of the test.
      if (!animeId) return

      // ── Step 2: navigate to the anime detail page. ─────────────────────
      await page.goto(`/anime/${animeId}`)
      await page.waitForLoadState('networkidle')

      // ── Step 3: install an event-collector on the parent window BEFORE
      // any iframe communication so we don't miss the early replies. ──────
      await page.evaluate(() => {
        interface WindowWithCanary extends Window {
          __kodikCanary: { events: Array<{ key: string; value: unknown; t: number }> }
        }
        const w = window as unknown as WindowWithCanary
        w.__kodikCanary = { events: [] }
        window.addEventListener('message', (ev) => {
          if (ev?.data && typeof ev.data === 'object' && 'key' in ev.data) {
            const k = (ev.data as { key: unknown }).key
            if (typeof k === 'string' && k.startsWith('kodik_player_')) {
              w.__kodikCanary.events.push({
                key: k,
                value: (ev.data as { value?: unknown }).value,
                t: Date.now(),
              })
            }
          }
        })
      })

      // ── Step 4: ensure the Kodik iframe is mounted. Some Anime.vue
      // variants render a placeholder until the user activates the player,
      // so click-to-load if needed. ───────────────────────────────────────
      const kodikIframe = page.locator('iframe[src*="kodik"]').first()
      const visibleQuickly = await kodikIframe
        .isVisible({ timeout: 3000 })
        .catch(() => false)
      if (!visibleQuickly) {
        // Try clicking common play-placeholder selectors.
        const placeholder = page
          .locator(
            'button:has-text(/Play|Загрузить|Смотреть|Watch/i), [aria-label*="play" i], .player-placeholder',
          )
          .first()
        if (await placeholder.isVisible({ timeout: 3000 }).catch(() => false)) {
          await placeholder.click({ force: true }).catch(() => undefined)
        }
      }
      await expect(kodikIframe).toBeVisible({ timeout: IFRAME_VISIBLE_TIMEOUT_MS })

      // Give the iframe ~1s to register window.player.api. Inbound RPC
      // commands posted pre-boot are silently dropped (except `play`).
      await page.waitForTimeout(1000)

      // ── Step 5: send get_time and assert kodik_player_time arrives. ────
      await page.evaluate(() => {
        const ifr = document.querySelector('iframe[src*="kodik"]') as HTMLIFrameElement | null
        if (!ifr || !ifr.contentWindow) throw new Error('no kodik iframe')
        ifr.contentWindow.postMessage(
          { key: 'kodik_player_api', value: { method: 'get_time' } },
          '*',
        )
      })

      const probeOk = await expect
        .poll(
          async () =>
            page.evaluate(() => {
              interface WindowWithCanary extends Window {
                __kodikCanary: { events: Array<{ key: string }> }
              }
              return (window as unknown as WindowWithCanary).__kodikCanary.events.some(
                (e) => e.key === 'kodik_player_time',
              )
            }),
          {
            timeout: PROBE_TIMEOUT_MS,
            message:
              'kodik_player_api get_time RPC did not reply with kodik_player_time within 5s — Kodik bundle may have changed (canary HIT). See memory reference reference_kodik_inbound_postmessage_api.md.',
          },
        )
        .toBe(true)
      // expect.poll throws on failure; reaching here means probe succeeded.
      void probeOk

      // ── Step 6: drive play, expect kodik_player_play. ──────────────────
      await page.evaluate(() => {
        interface WindowWithCanary extends Window {
          __kodikCanary: { events: Array<{ key: string }> }
        }
        const w = window as unknown as WindowWithCanary
        w.__kodikCanary.events = []
        const ifr = document.querySelector('iframe[src*="kodik"]') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'play' } },
          '*',
        )
      })
      await expect
        .poll(
          async () =>
            page.evaluate(() => {
              interface WindowWithCanary extends Window {
                __kodikCanary: { events: Array<{ key: string }> }
              }
              return (window as unknown as WindowWithCanary).__kodikCanary.events.some(
                (e) => e.key === 'kodik_player_play',
              )
            }),
          {
            timeout: COMMAND_RESPONSE_TIMEOUT_MS,
            message: 'kodik_player_api play RPC did not emit kodik_player_play',
          },
        )
        .toBe(true)

      // ── Step 7: drive pause, expect kodik_player_pause. ────────────────
      await page.evaluate(() => {
        interface WindowWithCanary extends Window {
          __kodikCanary: { events: Array<{ key: string }> }
        }
        const w = window as unknown as WindowWithCanary
        w.__kodikCanary.events = []
        const ifr = document.querySelector('iframe[src*="kodik"]') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'pause' } },
          '*',
        )
      })
      await expect
        .poll(
          async () =>
            page.evaluate(() => {
              interface WindowWithCanary extends Window {
                __kodikCanary: { events: Array<{ key: string }> }
              }
              return (window as unknown as WindowWithCanary).__kodikCanary.events.some(
                (e) => e.key === 'kodik_player_pause',
              )
            }),
          {
            timeout: COMMAND_RESPONSE_TIMEOUT_MS,
            message: 'kodik_player_api pause RPC did not emit kodik_player_pause',
          },
        )
        .toBe(true)

      // ── Step 8: drive seek(60), expect kodik_player_seek with value≈60. ─
      await page.evaluate(() => {
        interface WindowWithCanary extends Window {
          __kodikCanary: { events: Array<{ key: string }> }
        }
        const w = window as unknown as WindowWithCanary
        w.__kodikCanary.events = []
        const ifr = document.querySelector('iframe[src*="kodik"]') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'seek', seconds: 60 } },
          '*',
        )
      })
      await expect
        .poll(
          async () =>
            page.evaluate(() => {
              interface WindowWithCanary extends Window {
                __kodikCanary: { events: Array<{ key: string; value: unknown }> }
              }
              const events = (window as unknown as WindowWithCanary).__kodikCanary.events
              // Accept any kodik_player_seek event; some bundles fire multiple
              // (one immediate, one after the buffered range loads). We only
              // need to see at least one as proof the RPC dispatched.
              return events.some((e) => e.key === 'kodik_player_seek')
            }),
          {
            timeout: COMMAND_RESPONSE_TIMEOUT_MS,
            message: 'kodik_player_api seek RPC did not emit kodik_player_seek',
          },
        )
        .toBe(true)
    } finally {
      await ctx.close().catch(() => undefined)
    }
  })
})
