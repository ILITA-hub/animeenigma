import { test, expect, type Browser, type BrowserContext, type Page, type APIRequestContext } from '@playwright/test'

/**
 * Watch Together — two-browser sync end-to-end (WT-SYNC-09).
 *
 * Workstream watch-together / Phase 03 (player-sync) Plan 03.6.
 *
 * One test block per player kind. Each test:
 *   1. Creates a room with that player via POST /api/watch-together/rooms.
 *   2. Browser A and Browser B both navigate to invite_url.
 *   3. Browser A drives play/pause/seek.
 *   4. Browser B's state mirrors within 2s (500ms target + 4× slack).
 *
 * Plus one drift-correction test under simulated 4× CPU throttling on a
 * single player (aeplayer — HTML5 + cheap to drive). Verifies the soft
 * correction loop pulls the throttled follower back to the leader's
 * timeline once the throttle is released.
 *
 * Pre-flight gateway probe — the whole suite skips when the dev stack
 * isn't reachable. Mirrors the Phase 2 watch-together-shell.spec.ts
 * pattern (live-stack only). Run locally with:
 *
 *   cd frontend/web && bunx playwright test e2e/watch-together-sync.spec.ts --reporter=list
 *
 * Failure mode catalogue:
 *   - gateway down → whole suite skipped via test.beforeAll
 *   - no seeded watching anime → throws "no seeded anime found" with hint
 *     to re-run scripts/seed-ui-audit-user.sh
 *   - Kodik boot probe failed (banner visible on A) → kodik test skips its
 *     pause/seek tail (sync is disabled by design when the probe fails)
 *   - any room-creation 4xx → throws with the player kind in the message
 */

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

// 500ms target propagation × 4 slack for slow CI runners.
const SYNC_POLL_TIMEOUT_MS = 2000
const PLAYER_MOUNT_TIMEOUT_MS = 15_000

interface AuthResult {
  token: string
  user: Record<string, unknown>
}

interface RoomCreateResult {
  roomId: string
  inviteUrl: string
}

/**
 * Probe whether the watch-together backend is reachable. Same pattern as
 * watch-together-shell.spec.ts.
 */
async function isStackUp(request: APIRequestContext): Promise<boolean> {
  try {
    const resp = await request.get('/api/anime/_/scraper/health', { timeout: 3000 })
    if (!resp.ok()) return false
    const auth = await request.post('/api/auth/login', {
      data: { username: '__nonexistent_probe__', password: 'x' },
      timeout: 3000,
      failOnStatusCode: false,
    })
    return auth.status() > 0
  } catch {
    return false
  }
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
  return { token, user }
}

async function registerEphemeralUser(
  page: Page,
  request: APIRequestContext,
): Promise<AuthResult & { username: string }> {
  const username = `wt_sync_e2e_${Date.now()}_${Math.floor(Math.random() * 1000)}`
  const password = 'wt_sync_test_password_long_enough_for_validation_2026'
  const resp = await request.post('/api/auth/register', {
    data: { username, password, confirm_password: password },
  })
  if (!resp.ok()) {
    throw new Error(`register(${username}) failed: ${resp.status()} ${await resp.text()}`)
  }
  const body = await resp.json()
  const data = body?.data ?? body
  const token: string | undefined = data?.access_token
  const user: Record<string, unknown> | undefined = data?.user
  if (!token || !user) {
    throw new Error('register: no token/user in response')
  }
  await page.addInitScript(
    ({ tok, usr }) => {
      window.localStorage.setItem('token', tok)
      window.localStorage.setItem('user', JSON.stringify(usr))
    },
    { tok: token, usr: user },
  )
  return { token, user, username }
}

let cachedAnimeId: string | null = null

async function resolveSeededAnimeId(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  if (cachedAnimeId) return cachedAnimeId
  // Player router exposes the watchlist at `/api/users/watchlist`; the
  // old `/me/anime-list` path was never wired up. Envelope is { success, data: [...] }.
  const resp = await request.get('/api/users/watchlist?status=watching', {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!resp.ok()) {
    throw new Error(`could not fetch seeded watchlist: ${resp.status()}`)
  }
  const body = await resp.json()
  const items = body?.data ?? body?.items ?? []
  if (!Array.isArray(items) || items.length === 0) {
    throw new Error(
      'no seeded anime found — run scripts/seed-ui-audit-user.sh to seed ui_audit_bot',
    )
  }
  const first = items[0]
  const id: string | undefined = first?.anime_id ?? first?.anime?.id ?? first?.id
  if (!id) throw new Error('anime_list row has no anime_id/anime.id')
  cachedAnimeId = id
  return id
}

async function createRoom(
  request: APIRequestContext,
  token: string,
  animeId: string,
  player: string,
): Promise<RoomCreateResult> {
  const resp = await request.post('/api/watch-together/rooms', {
    headers: { Authorization: `Bearer ${token}` },
    // Backend rejects empty translation_id with INVALID_INPUT (rooms.go:83-84).
    // For sync tests we just need a non-empty value — the player is the unit
    // under test, not provider-specific translation resolution.
    data: { anime_id: animeId, episode_id: '1', player, translation_id: 'e2e-seeded' },
  })
  if (!resp.ok()) {
    throw new Error(
      `create room failed for player=${player}: ${resp.status()} ${await resp.text()}`,
    )
  }
  const body = await resp.json()
  const data = body?.data ?? body
  const roomId: string | undefined = data?.room_id ?? data?.id
  const inviteUrl: string | undefined = data?.invite_url
  if (!roomId || !inviteUrl) {
    throw new Error(`create room: response missing room_id/invite_url for player=${player}`)
  }
  return { roomId, inviteUrl }
}

interface PairSetup {
  ctxA: BrowserContext
  ctxB: BrowserContext
  pageA: Page
  pageB: Page
  inviteUrl: string
}

async function setupPair(
  browser: Browser,
  request: APIRequestContext,
  player: string,
): Promise<PairSetup> {
  const ctxA = await browser.newContext()
  const ctxB = await browser.newContext()
  const pageA = await ctxA.newPage()
  const pageB = await ctxB.newPage()

  const authA = await loginAs(pageA, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)
  await registerEphemeralUser(pageB, request)

  const animeId = await resolveSeededAnimeId(request, authA.token)
  const { inviteUrl } = await createRoom(request, authA.token, animeId, player)

  await pageA.goto(inviteUrl)
  await pageB.goto(inviteUrl)
  await pageA.waitForURL(/\/watch\/room\//, { timeout: 10_000 })
  await pageB.waitForURL(/\/watch\/room\//, { timeout: 10_000 })

  return { ctxA, ctxB, pageA, pageB, inviteUrl }
}

async function teardownPair(pair: Pick<PairSetup, 'ctxA' | 'ctxB'>): Promise<void> {
  await pair.ctxA.close().catch(() => undefined)
  await pair.ctxB.close().catch(() => undefined)
}

test.describe('Watch Together — two-browser sync (WT-SYNC-09, Plan 03.6)', () => {
  test.beforeAll(async ({ request }) => {
    const up = await isStackUp(request)
    test.skip(!up, 'AnimeEnigma local stack is not up — run `make dev` and re-run')
  })

  // ─── HTML5 player (aePlayer) ───────────────────────────────────────────
  //
  // After the Plan B player-surface collapse, aePlayer is the sole HTML5
  // <video> surface (it serves every source — RU/EN/JP/18+ — internally).
  // The block drives play/pause/seek on browser A and asserts browser B
  // mirrors within SYNC_POLL_TIMEOUT_MS. The <video> element is the
  // single source of truth on both sides.

  for (const player of ['aeplayer'] as const) {
    test(`sync: two browsers stay in lockstep on ${player} player`, async ({
      browser,
      request,
    }) => {
      const pair = await setupPair(browser, request, player)
      const { pageA, pageB } = pair
      try {
        const videoA = pageA.locator('video').first()
        const videoB = pageB.locator('video').first()
        await expect(videoA).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })
        await expect(videoB).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })

        // Play in A → B's video should be unpaused within 2s.
        await videoA.evaluate((v: HTMLVideoElement) => v.play().catch(() => undefined))
        await expect
          .poll(
            async () => videoB.evaluate((v: HTMLVideoElement) => !v.paused),
            {
              timeout: SYNC_POLL_TIMEOUT_MS,
              message: `B did not start playing after A played [${player}]`,
            },
          )
          .toBe(true)

        // Pause in A → B paused within 2s.
        await videoA.evaluate((v: HTMLVideoElement) => v.pause())
        await expect
          .poll(
            async () => videoB.evaluate((v: HTMLVideoElement) => v.paused),
            {
              timeout: SYNC_POLL_TIMEOUT_MS,
              message: `B did not pause after A paused [${player}]`,
            },
          )
          .toBe(true)

        // Seek in A to 30s → B currentTime ≈ 30s ±1s within 2s.
        await videoA.evaluate((v: HTMLVideoElement) => {
          v.currentTime = 30
        })
        await expect
          .poll(
            async () => videoB.evaluate((v: HTMLVideoElement) => v.currentTime),
            {
              timeout: SYNC_POLL_TIMEOUT_MS,
              message: `B did not seek to ~30s after A seeked [${player}]`,
            },
          )
          .toBeGreaterThan(29)
        const finalTimeB = await videoB.evaluate((v: HTMLVideoElement) => v.currentTime)
        expect(finalTimeB, `B final currentTime drifted past 32s on ${player}`).toBeLessThan(32)
      } finally {
        await teardownPair(pair)
      }
    })
  }

  // ─── Kodik (iframe postMessage RPC) ────────────────────────────────────
  //
  // Kodik exposes no DOM-level <video>; sync travels via the
  // `kodik_player_api` postMessage RPC discovered 2026-05-25 and wired in
  // Plan 03.4. Browser A posts `play`/`pause` to its iframe; the parent
  // bridge on B receives the corresponding `kodik_player_play` /
  // `kodik_player_pause` event from B's iframe (the bridge drives B's
  // iframe via postCommand → which then echoes back the same event keys).
  //
  // If the Kodik boot probe failed on A (banner visible), sync is
  // explicitly disabled by design. We assert the play assertion still
  // exercises the bridge, then skip the pause/seek tail with a note.

  test('sync: two browsers stay in lockstep on kodik player', async ({ browser, request }) => {
    const pair = await setupPair(browser, request, 'kodik')
    const { pageA, pageB } = pair
    try {
      const ifrA = pageA.locator('iframe').first()
      const ifrB = pageB.locator('iframe').first()
      await expect(ifrA).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })
      await expect(ifrB).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })

      // Install message collector on B BEFORE driving A, so we don't
      // miss the early echo. Keys we care about all share the
      // `kodik_player_` prefix.
      await pageB.evaluate(() => {
        interface WindowWithSync extends Window {
          __wtSyncEvents: string[]
        }
        const w = window as unknown as WindowWithSync
        w.__wtSyncEvents = []
        window.addEventListener('message', (ev) => {
          const data = ev?.data as { key?: unknown } | null
          const key = data?.key
          if (typeof key === 'string' && key.startsWith('kodik_player_')) {
            w.__wtSyncEvents.push(key)
          }
        })
      })

      // Wait ~1s for both iframes' window.player.api to register.
      // Inbound RPC commands posted pre-boot are silently dropped.
      await pageA.waitForTimeout(1000)

      // Drive play in A via RPC.
      await pageA.evaluate(() => {
        const ifr = document.querySelector('iframe') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'play' } },
          '*',
        )
      })

      // Assert B's iframe emitted kodik_player_play (driven by the bridge
      // → postCommand('play') on B's iframe → echo as kodik_player_play).
      await expect
        .poll(
          async () =>
            pageB.evaluate(() => {
              interface WindowWithSync extends Window {
                __wtSyncEvents: string[]
              }
              return (window as unknown as WindowWithSync).__wtSyncEvents.includes(
                'kodik_player_play',
              )
            }),
          {
            timeout: SYNC_POLL_TIMEOUT_MS,
            message: 'B did not receive kodik_player_play after A played',
          },
        )
        .toBe(true)

      // If the boot probe failed (banner visible on A) the rest of the
      // sync surface is disabled by design. Detect via the kodik_sync_unavailable
      // banner and bail with a structured note.
      const fallbackBanner = pageA.getByText(
        /Kodik sync unavailable|Синхронизация Kodik недоступна/i,
      )
      const fallbackVisible = await fallbackBanner.isVisible({ timeout: 500 }).catch(() => false)
      if (fallbackVisible) {
        test.info().annotations.push({
          type: 'note',
          description:
            'Kodik boot probe failed on A — pause/seek assertions skipped (sync disabled by design)',
        })
        return
      }

      // Drive pause in A → B emits kodik_player_pause.
      await pageB.evaluate(() => {
        interface WindowWithSync extends Window {
          __wtSyncEvents: string[]
        }
        const w = window as unknown as WindowWithSync
        w.__wtSyncEvents = []
      })
      await pageA.evaluate(() => {
        const ifr = document.querySelector('iframe') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'pause' } },
          '*',
        )
      })
      await expect
        .poll(
          async () =>
            pageB.evaluate(() => {
              interface WindowWithSync extends Window {
                __wtSyncEvents: string[]
              }
              return (window as unknown as WindowWithSync).__wtSyncEvents.includes(
                'kodik_player_pause',
              )
            }),
          {
            timeout: SYNC_POLL_TIMEOUT_MS,
            message: 'B did not receive kodik_player_pause after A paused',
          },
        )
        .toBe(true)

      // Drive seek(30) in A → B emits kodik_player_seek.
      await pageB.evaluate(() => {
        interface WindowWithSync extends Window {
          __wtSyncEvents: string[]
        }
        const w = window as unknown as WindowWithSync
        w.__wtSyncEvents = []
      })
      await pageA.evaluate(() => {
        const ifr = document.querySelector('iframe') as HTMLIFrameElement | null
        ifr?.contentWindow?.postMessage(
          { key: 'kodik_player_api', value: { method: 'seek', seconds: 30 } },
          '*',
        )
      })
      await expect
        .poll(
          async () =>
            pageB.evaluate(() => {
              interface WindowWithSync extends Window {
                __wtSyncEvents: string[]
              }
              return (window as unknown as WindowWithSync).__wtSyncEvents.includes(
                'kodik_player_seek',
              )
            }),
          {
            timeout: SYNC_POLL_TIMEOUT_MS,
            message: 'B did not receive kodik_player_seek after A seeked',
          },
        )
        .toBe(true)
    } finally {
      await teardownPair(pair)
    }
  })

  // ─── Drift correction under CPU throttling (aeplayer) ──────────────────
  //
  // Throttle B's tab to 4× CPU slowdown, let A play unthrottled for a few
  // seconds (B accumulates drift since its video clock advances slower
  // than A's). Release the throttle and assert the soft correction loop
  // pulls B's currentTime back within 3s of A's within a 3-second
  // observation window. The soft window is 5s in the implementation; we
  // assert <3s convergence to leave headroom.

  test('sync: drift correction pulls B back when throttled (aeplayer)', async ({
    browser,
    request,
  }) => {
    const pair = await setupPair(browser, request, 'aeplayer')
    const { pageA, pageB } = pair
    try {
      const videoA = pageA.locator('video').first()
      const videoB = pageB.locator('video').first()
      await expect(videoA).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })
      await expect(videoB).toBeVisible({ timeout: PLAYER_MOUNT_TIMEOUT_MS })

      // Open a CDP session on B and throttle CPU 4×.
      const cdp = await pageB.context().newCDPSession(pageB)
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 })

      // Kick both videos into play.
      await videoA.evaluate((v: HTMLVideoElement) => v.play().catch(() => undefined))
      await videoB.evaluate((v: HTMLVideoElement) => v.play().catch(() => undefined))

      // Let drift accumulate (~6s of throttled play on B).
      await pageA.waitForTimeout(6000)

      // Release the throttle and give the soft correction loop 3s to act.
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 })
      await pageA.waitForTimeout(3000)

      const [tA, tB] = await Promise.all([
        videoA.evaluate((v: HTMLVideoElement) => v.currentTime),
        videoB.evaluate((v: HTMLVideoElement) => v.currentTime),
      ])
      // Convergence within 3 seconds of each other.
      expect(
        Math.abs(tA - tB),
        `drift between A (${tA}s) and B (${tB}s) exceeded 3s after un-throttle`,
      ).toBeLessThan(3)
    } finally {
      await teardownPair(pair)
    }
  })
})
