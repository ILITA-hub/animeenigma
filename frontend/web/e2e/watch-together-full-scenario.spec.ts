import { test, expect, type Page, type APIRequestContext } from '@playwright/test'

/**
 * Workstream watch-together — comprehensive 2-browser e2e scenario.
 *
 * The previous version of this spec seeded `pref:<animeId>` localStorage
 * to short-circuit the button's translation resolution, which silently
 * masked the bug the user actually hits: a cold-clicker without a cached
 * preference was getting a 12s wait followed by a red error toast.
 *
 * This version clicks the button COLD — no localStorage seeding, the same
 * way a real user encounters the feature. The whole scenario fits in one
 * test to keep both browsers under the per-user RPM rate limit (which is
 * easy to blow through with back-to-back test runs against the same bot).
 *
 * Flow:
 *   1. A logs in (ui_audit_bot), B registers an ephemeral user
 *   2. Pick an anime that has kodik translations (cache it across runs)
 *   3. A visits anime page, clicks Invite cold
 *   4. URL changes to /watch/room/<uuid> within 2.5s (was 12s+ before)
 *   5. No red error toast on success path
 *   6. B opens the same URL → both see member count = 2 + each other's name
 *   7. Chat A → B, chat B → A
 *   8. Reaction A → B, reaction B → A
 *   9. Controls: 2 player tabs (aeplayer + kodik) on each side, both agree on active tab
 *  10. A reloads → still in room (snapshot replay)
 *  11. B closes → A drops back to 1 member
 *
 * Stack assumptions (`make health` green):
 *   - web on http://localhost:3003 (BASE_URL env)
 *   - gateway, auth, catalog, player, watch-together:8091, redis up
 *   - `ui_audit_bot` seeded with ≥1 watching row + that anime has kodik
 *     translations (catalog `/api/anime/{id}/kodik/translations` ≥ 1)
 *
 * Run:
 *   bunx playwright test e2e/watch-together-full-scenario.spec.ts \
 *     --project=chromium --reporter=list --workers=1
 */

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

const MEMBER_LIST = '[data-testid="wt-member-entry"]'
const INVITE_BUTTON = '[data-testid="wt-invite-button"]'
const CHAT_INPUT =
  'textarea[aria-label*="chat" i], textarea[aria-label*="сообщ" i], textarea[placeholder*="message" i], textarea[placeholder*="сообщ" i]'
const PLAYER_TAB = 'button[role="tab"][data-player]'

interface AuthResult {
  token: string
  user: { id: string; username: string; [k: string]: unknown }
}

async function loginAs(
  page: Page,
  request: APIRequestContext,
  username: string,
  password: string,
): Promise<AuthResult> {
  const resp = await request.post('/api/auth/login', {
    data: { username, password },
  })
  expect(resp.ok(), `login(${username}) http`).toBeTruthy()
  const body = await resp.json()
  const data = body?.data ?? body
  const token: string | undefined = data?.access_token
  const user = data?.user as AuthResult['user'] | undefined
  if (!token || !user) throw new Error(`login(${username}): missing token/user`)
  await page.addInitScript(
    ({ tok, usr }) => {
      window.localStorage.setItem('token', tok)
      window.localStorage.setItem('user', JSON.stringify(usr))
    },
    { tok: token, usr: user },
  )
  return { token, user }
}

async function registerEphemeralUser(
  page: Page,
  request: APIRequestContext,
): Promise<AuthResult> {
  const username = `wt_full_${Date.now()}_${Math.floor(Math.random() * 1000)}`
  const password = 'wt_full_password_long_enough_for_validation_2026'
  const resp = await request.post('/api/auth/register', {
    data: { username, password, confirm_password: password },
  })
  expect(resp.ok(), `register(${username}) http`).toBeTruthy()
  const body = await resp.json()
  const data = body?.data ?? body
  const token: string | undefined = data?.access_token
  const user = data?.user as AuthResult['user'] | undefined
  if (!token || !user) throw new Error(`register(${username}): missing token/user`)
  await page.addInitScript(
    ({ tok, usr }) => {
      window.localStorage.setItem('token', tok)
      window.localStorage.setItem('user', JSON.stringify(usr))
    },
    { tok: token, usr: user },
  )
  return { token, user }
}

/**
 * Pick an anime that has at least one kodik translation. The InviteButton
 * fetches translations on click and needs ≥1 row to mint a real
 * translation_id. Cached across the test file's lifetime so re-runs
 * don't re-iterate the bot's watchlist (which burns rate-limit budget).
 */
let CACHED_ANIME_ID: string | null = null
async function pickAnimeWithKodikTranslations(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  if (CACHED_ANIME_ID) return CACHED_ANIME_ID
  const listResp = await request.get('/api/users/watchlist?status=watching', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(listResp.ok(), 'watchlist fetch http').toBeTruthy()
  const body = await listResp.json()
  const items: Array<Record<string, unknown>> = body?.data ?? body?.items ?? []
  expect(items.length, 'bot has watching rows').toBeGreaterThan(0)

  for (const row of items) {
    const r = row as { anime_id?: string; anime?: { id?: string } }
    const id = r.anime_id ?? r.anime?.id
    if (!id) continue
    const trResp = await request.get(`/api/anime/${id}/kodik/translations`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!trResp.ok()) continue
    const tr = await trResp.json()
    const rows = (tr?.data ?? tr ?? []) as unknown[]
    if (Array.isArray(rows) && rows.length > 0) {
      CACHED_ANIME_ID = id
      return id
    }
  }
  throw new Error(
    'no seeded watching anime has kodik translations — re-run scripts/seed-ui-audit-user.sh',
  )
}

test.describe('Watch Together — full 2-browser scenario', () => {
  test.setTimeout(180_000)

  test('cold click → 2 browsers → chat, reactions, controls, reload, leave', async ({
    browser,
    request,
  }) => {
    const ctxA = await browser.newContext()
    const ctxB = await browser.newContext()
    try {
      const pageA = await ctxA.newPage()
      const pageB = await ctxB.newPage()

      // Surface failing watch-together responses so a regression to the
      // 410-Gone race shows up as a clear log line in test output.
      const surface = (page: Page, tag: string) => {
        page.on('response', (resp) => {
          const u = resp.url()
          if (u.includes('/api/watch-together/') && resp.status() >= 400) {
            // eslint-disable-next-line no-console
            console.log(`[${tag}] ${resp.status()} ${u}`)
          }
        })
      }
      surface(pageA, 'A')
      surface(pageB, 'B')

      // ── 1. Auth setup ─────────────────────────────────────────────
      const authA = await loginAs(pageA, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)
      const authB = await registerEphemeralUser(pageB, request)
      const animeId = await pickAnimeWithKodikTranslations(request, authA.token)

      // Wipe any pref:* localStorage on A — the click MUST go through the
      // cold-click path (catalog fetch on click, no preference cache).
      await pageA.addInitScript(() => {
        try {
          for (let i = window.localStorage.length - 1; i >= 0; i--) {
            const key = window.localStorage.key(i)
            if (key && key.startsWith('pref:')) {
              window.localStorage.removeItem(key)
            }
          }
        } catch {
          // privacy mode — nothing to clean
        }
      })

      // ── 2. A visits anime, clicks Invite cold ─────────────────────
      await pageA.goto(`/anime/${animeId}`)
      await pageA.waitForLoadState('domcontentloaded')

      // No pre-click error toast.
      const errorToast = pageA.locator('[role="alert"]', {
        hasText: /Couldn't create the room|Не удалось создать комнату/i,
      })
      expect(await errorToast.count(), 'no pre-click error toast').toBe(0)

      const btn = pageA.locator(INVITE_BUTTON)
      await expect(btn, 'InviteButton visible above the fold').toBeVisible({
        timeout: 10_000,
      })

      // Time the click → URL change. Budget 2.5s — the new
      // catalog-fetch path is typically <300ms. The prior 12s player-
      // chain wait would fail this budget by an order of magnitude.
      const t0 = Date.now()
      await btn.click()
      await pageA.waitForURL(/\/watch\/room\/[a-f0-9-]{8,}/, { timeout: 2_500 })
      const elapsed = Date.now() - t0
      // eslint-disable-next-line no-console
      console.log(`InviteButton click → room URL took ${elapsed}ms`)
      expect(elapsed, 'click → URL change budget').toBeLessThan(2_500)

      const roomUrl = pageA.url()
      expect(roomUrl.match(/\/watch\/room\/([a-f0-9-]+)/)?.[1], 'room id').toMatch(
        /[a-f0-9-]{8,}/,
      )

      // No error toast after the click either.
      await pageA.waitForTimeout(500)
      expect(await errorToast.count(), 'no error toast after click').toBe(0)

      // A sees self in member list.
      await expect(pageA.locator(MEMBER_LIST), 'A self-row').toHaveCount(1, {
        timeout: 12_000,
      })

      // ── 3. B opens the URL → both see 2 members ──────────────────
      await pageB.goto(roomUrl)
      await pageB.waitForLoadState('domcontentloaded')
      await expect(pageB.locator(MEMBER_LIST), 'B member list').toHaveCount(2, {
        timeout: 15_000,
      })
      await expect(pageA.locator(MEMBER_LIST), 'A member list').toHaveCount(2, {
        timeout: 15_000,
      })

      // Both surface both usernames.
      const sidebarA = pageA.locator('aside').first()
      const sidebarB = pageB.locator('aside').first()
      await expect(sidebarA).toContainText(authA.user.username)
      await expect(sidebarA).toContainText(authB.user.username)
      await expect(sidebarB).toContainText(authA.user.username)
      await expect(sidebarB).toContainText(authB.user.username)

      // ── 4. Chat A → B ────────────────────────────────────────────
      const chatA = pageA.locator(CHAT_INPUT).first()
      await expect(chatA).toBeVisible({ timeout: 8_000 })
      const msgA = `hello from A ${Date.now()}`
      await chatA.fill(msgA)
      await chatA.press('Enter')
      await expect(pageB.locator(`text=${msgA}`).first()).toBeVisible({
        timeout: 8_000,
      })

      // ── 5. Chat B → A ────────────────────────────────────────────
      const chatB = pageB.locator(CHAT_INPUT).first()
      const msgB = `hi A ${Date.now()}`
      await chatB.fill(msgB)
      await chatB.press('Enter')
      await expect(pageA.locator(`text=${msgB}`).first()).toBeVisible({
        timeout: 8_000,
      })

      // ── 6. Reactions A → B and B → A ─────────────────────────────
      await pageA.locator('button[aria-label="🔥"]').first().click()
      await expect(pageB.locator('span:has-text("🔥")').first()).toBeVisible({
        timeout: 8_000,
      })
      await pageB.locator('button[aria-label="🎉"]').first().click()
      await expect(pageA.locator('span:has-text("🎉")').first()).toBeVisible({
        timeout: 8_000,
      })

      // ── 7. Controls: 2 player tabs (aeplayer + kodik, the Plan B
      //       survivors), both agree on active tab ─────
      await expect(pageA.locator(PLAYER_TAB)).toHaveCount(2)
      await expect(pageB.locator(PLAYER_TAB)).toHaveCount(2)
      const activeA = await pageA
        .locator(`${PLAYER_TAB}[aria-selected="true"]`)
        .first()
        .getAttribute('data-player')
      const activeB = await pageB
        .locator(`${PLAYER_TAB}[aria-selected="true"]`)
        .first()
        .getAttribute('data-player')
      expect(activeA).toBeTruthy()
      expect(activeB).toBe(activeA)

      // ── 8. A reloads → still in room ─────────────────────────────
      await pageA.reload({ waitUntil: 'domcontentloaded' })
      await pageA.waitForURL(/\/watch\/room\/[a-f0-9-]{8,}/, { timeout: 10_000 })
      await expect(pageA.locator(MEMBER_LIST)).toHaveCount(2, { timeout: 15_000 })

      // ── 9. B closes → A drops to 1 ───────────────────────────────
      await ctxB.close()
      await expect(pageA.locator(MEMBER_LIST), 'A drops to 1').toHaveCount(1, {
        timeout: 30_000,
      })
    } finally {
      await ctxA.close().catch(() => undefined)
      await ctxB.close().catch(() => undefined)
    }
  })
})
