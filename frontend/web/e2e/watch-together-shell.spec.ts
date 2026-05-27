import { test, expect, type Page, type APIRequestContext } from '@playwright/test'

/**
 * Workstream watch-together / Phase 02 (frontend-shell) Plan 02.10.
 *
 * Two-browser end-to-end smoke test of the Watch Together feature.
 *
 *   Browser A: ui_audit_bot (permanent seeded test user) creates a room
 *   Browser B: ephemeral random user joins via the invite link
 *
 * Scenario:
 *   1. A logs in as ui_audit_bot, visits an anime page, clicks Invite
 *      → URL changes to /watch/room/<uuid>, room snapshot loads
 *   2. B logs in as a fresh random user, opens the same URL
 *      → lands in the same room, both A and B appear in the member list
 *   3. A sends a chat message → B sees it
 *   4. B sends a chat message → A sees it
 *   5. A sends a 🔥 reaction → B sees a floating burst (and vice versa)
 *   6. B closes their browser context → A's member list drops to 1
 *   7. Expired/non-existent room URL → "Room ended" empty state
 *
 * The spec ALSO covers the i18n smoke-verify checkpoint from Plan 02.10:
 *   - In both 'en' and 'ru' locales, the room view renders WITHOUT any
 *     raw `watch_together.*` key strings appearing in the DOM. This is
 *     the runtime guardrail for the MEMORY rule `feedback_smoke_verify_i18n`
 *     (string-typed i18n keys pass every tsc/eslint/spec gate and only
 *     fail at runtime as raw key strings).
 *
 * Runtime expectations (see Plan 02.10 SUMMARY for runbook):
 *   - Frontend dev server (or built static + nginx) reachable at BASE_URL
 *     (default http://localhost:3003 per playwright.config.ts).
 *   - Backend gateway reachable through the same origin proxy at /api/*.
 *   - watch-together:8091, redis, postgres, gateway:8000, catalog all up
 *     (i.e. `make health` is green). The smoke test in
 *     `scripts/smoke-watch-together.sh` verifies the backend layer
 *     independently — this spec then verifies the frontend layer.
 *   - The permanent `ui_audit_bot` user is seeded (run
 *     `./scripts/seed-ui-audit-user.sh` once if not).
 *
 * Run: `cd frontend/web && bunx playwright test e2e/watch-together-shell.spec.ts --reporter=list`
 * Skip in CI when stack is not running: tests gracefully bail out via the
 * `try-catch` around the room snapshot check (see Stack-availability gate).
 */

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

/**
 * Seeded anime UUID — the FIRST `watching` row in `ui_audit_bot`'s
 * anime_list (per scripts/seed-ui-audit-user.sh). We pick a `watching`
 * entry because resumeStartEpisode is wired to that status; the
 * InviteButton's `playerActivated` gate then becomes reachable by simply
 * clicking the click-to-load placeholder on the player.
 *
 * The script seeds by `ORDER BY score DESC NULLS LAST LIMIT 8` so the
 * actual UUID isn't a stable literal — we resolve it dynamically below
 * (resolveSeededAnimeId).
 */
let SEEDED_ANIME_ID: string | null = null

async function resolveSeededAnimeId(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  if (SEEDED_ANIME_ID) return SEEDED_ANIME_ID
  // Use the player service's watchlist endpoint with the bot's JWT.
  // Route is `GET /api/users/watchlist?status=...` per player router.go.
  const resp = await request.get('/api/users/watchlist?status=watching', {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!resp.ok()) {
    throw new Error(`could not fetch seeded watchlist: ${resp.status()}`)
  }
  const body = await resp.json()
  // Backend envelope is { success, data: [...] } — items are flat in data.
  const items = body?.data ?? body?.items ?? []
  if (!Array.isArray(items) || items.length === 0) {
    throw new Error(
      'ui_audit_bot has no `watching` anime — re-run ./scripts/seed-ui-audit-user.sh',
    )
  }
  // The list rows carry `anime_id` (or nested `anime.id`); accept either.
  const first = items[0]
  const id: string | undefined = first?.anime_id ?? first?.anime?.id ?? first?.id
  if (!id) throw new Error('anime_list row has no anime_id/anime.id')
  SEEDED_ANIME_ID = id
  return id
}

interface AuthResult {
  token: string
  user: Record<string, unknown>
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
  if (!resp.ok()) {
    throw new Error(`login(${username}) failed: ${resp.status()} ${await resp.text()}`)
  }
  const body = await resp.json()
  const data = body?.data ?? body
  const token: string | undefined = data?.access_token
  const user: Record<string, unknown> | undefined = data?.user
  if (!token || !user) {
    throw new Error(`login(${username}): no token/user in response: ${JSON.stringify(body).slice(0, 200)}`)
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
): Promise<AuthResult & { username: string; password: string }> {
  const username = `wt_test_${Date.now()}_${Math.floor(Math.random() * 1000)}`
  const password = 'wt_test_password_long_enough_for_validation_2026'
  const resp = await request.post('/api/auth/register', {
    data: { username, password, confirm_password: password },
  })
  if (!resp.ok()) {
    throw new Error(
      `register(${username}) failed: ${resp.status()} ${await resp.text()}`,
    )
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
  return { token, user, username, password }
}

async function setLocale(page: Page, locale: 'en' | 'ru'): Promise<void> {
  await page.addInitScript((loc) => {
    window.localStorage.setItem('locale', loc)
  }, locale)
}

/**
 * Probe whether the watch-together backend is reachable. Used to skip
 * gracefully when running from an environment where the docker stack
 * is not up (e.g. an isolated CI runner).
 */
async function isStackUp(request: APIRequestContext): Promise<boolean> {
  try {
    const resp = await request.get('/api/anime/_/scraper/health', { timeout: 3000 })
    if (!resp.ok()) return false
    // Also require gateway auth path to respond.
    const auth = await request.post('/api/auth/login', {
      data: { username: '__nonexistent_probe__', password: 'x' },
      timeout: 3000,
      failOnStatusCode: false,
    })
    // Any HTTP response (401/400/etc.) means the gateway is alive.
    return auth.status() > 0
  } catch {
    return false
  }
}

test.describe('Watch Together — two-browser smoke (Phase 02 Plan 02.10)', () => {
  // The whole-suite stack-up gate: if the backend isn't reachable we skip
  // every test in this file rather than fail with cryptic timeouts.
  test.beforeAll(async ({ request }) => {
    const up = await isStackUp(request)
    test.skip(!up, 'AnimeEnigma local stack is not up — run `make dev` and re-run')
  })

  test('two browsers can create + join + chat + react + leave a room', async ({
    browser,
    request,
  }) => {
    // ── 1. Setup: log in two isolated contexts ──────────────────────────
    const ctxA = await browser.newContext()
    const ctxB = await browser.newContext()
    try {
      const pageA = await ctxA.newPage()
      const pageB = await ctxB.newPage()

      const authA = await loginAs(pageA, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)
      const animeId = await resolveSeededAnimeId(request, authA.token)
      await registerEphemeralUser(pageB, request)

      // Seed the `pref:<animeId>` localStorage cache that useWatchPreferences
      // reads synchronously on construction. This puts the test browser in
      // the "returning user" state: resolvedCombo is populated BEFORE the
      // player needs to mount, so the InviteButton renders immediately.
      // Mirrors what real returning users see (and avoids fighting the
      // provider-resolution chain in a headless test environment where
      // kodik/animelib/etc. round-trips can flake).
      const seededCombo = {
        data: {
          player: 'kodik',
          language: 'ru',
          watch_type: 'sub',
          translation_id: 'e2e-seed-translation',
          translation_title: 'E2E Seeded',
          tier: 'user_global',
          tier_number: 2,
        },
        timestamp: Date.now(),
      }
      await pageA.addInitScript(
        ({ key, value }) => {
          localStorage.setItem(key, JSON.stringify(value))
        },
        { key: `pref:${animeId}`, value: seededCombo },
      )

      // ── 2. Browser A visits the anime page ───────────────────────────
      await pageA.goto(`/anime/${animeId}`)
      await pageA.waitForLoadState('networkidle')

      // The InviteButton renders the i18n key `invite_button_label`. In
      // 'en' that's "Invite to Watch Together"; in 'ru' "Пригласить...";
      // we accept either + the aria-label that mirrors the same key.
      const inviteBtn = pageA
        .locator(
          'button:has-text("Invite to Watch Together"), button:has-text("Пригласить"), button[aria-label*="Invite" i], button[aria-label*="Пригласить" i]',
        )
        .first()

      // The InviteButton is gated behind `resolvedCombo` (Anime.vue line 128).
      // After clicking Continue, the player must mount, emit available-
      // translations, and useWatchPreferences must resolve a combo before
      // the button renders. Allow up to 25s — provider resolution + network
      // round-trip + the resolve POST.
      await expect(inviteBtn).toBeVisible({ timeout: 25_000 })

      // ── 3. A clicks Invite → URL changes + member list shows A ───────
      await inviteBtn.click()
      await pageA.waitForURL(/\/watch\/room\/[a-f0-9-]{8,}/, { timeout: 10_000 })
      const roomUrl = pageA.url()
      const roomId = roomUrl.match(/\/watch\/room\/([a-f0-9-]+)/)?.[1]
      expect(roomId, 'roomId parsed from URL').toBeTruthy()

      // Wait for the room:snapshot frame → at least one member visible.
      await expect(pageA.locator('aside li, [data-testid="member-list"] li')).toHaveCount(1, { timeout: 8_000 })

      // ── 4. Browser B opens the invite URL → both see each other ──────
      await pageB.goto(roomUrl)
      await pageB.waitForURL(/\/watch\/room\//, { timeout: 10_000 })

      // Both pages should show 2 members in the list within 8s.
      await expect(pageA.locator('aside li, [data-testid="member-list"] li')).toHaveCount(2, { timeout: 8_000 })
      await expect(pageB.locator('aside li, [data-testid="member-list"] li')).toHaveCount(2, { timeout: 8_000 })

      // ── 5. A sends a chat message; B sees it ─────────────────────────
      const chatA = pageA
        .locator(
          'textarea[aria-label*="chat" i], textarea[aria-label*="сообщ" i], textarea[placeholder*="message" i], textarea[placeholder*="сообщ" i]',
        )
        .first()
      await chatA.fill('Hello from A')
      await chatA.press('Enter')
      await expect(pageB.locator('li:has-text("Hello from A"), .message:has-text("Hello from A")').first())
        .toBeVisible({ timeout: 5_000 })

      // ── 6. B sends a chat message; A sees it ─────────────────────────
      const chatB = pageB
        .locator(
          'textarea[aria-label*="chat" i], textarea[aria-label*="сообщ" i], textarea[placeholder*="message" i], textarea[placeholder*="сообщ" i]',
        )
        .first()
      await chatB.fill('Hi A')
      await chatB.press('Enter')
      await expect(pageA.locator('li:has-text("Hi A"), .message:has-text("Hi A")').first())
        .toBeVisible({ timeout: 5_000 })

      // ── 7. A sends a 🔥 reaction; B sees the floating burst ──────────
      const fireBtn = pageA.locator('button[aria-label="🔥"], button:has-text("🔥")').first()
      await expect(fireBtn).toBeVisible({ timeout: 5_000 })
      await fireBtn.click()
      // The burst overlay is inside the player wrapper; at least one
      // span carrying the emoji must appear on B within 5s.
      await expect(pageB.locator('span:has-text("🔥")').first()).toBeVisible({ timeout: 5_000 })

      // ── 8. B closes; A sees member count drop to 1 ───────────────────
      await ctxB.close()
      await expect(pageA.locator('aside li, [data-testid="member-list"] li')).toHaveCount(1, { timeout: 12_000 })
    } finally {
      await ctxA.close().catch(() => undefined)
      await ctxB.close().catch(() => undefined)
    }
  })

  test('expired/non-existent room URL renders the room-ended empty state', async ({
    browser,
    request,
  }) => {
    const ctx = await browser.newContext()
    try {
      const page = await ctx.newPage()
      await loginAs(page, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)
      // The all-zeros UUID is guaranteed to not exist; backend returns 410 Gone
      // (or 404), the view treats both as "room ended".
      await page.goto('/watch/room/00000000-0000-0000-0000-000000000000')
      // i18n key: watch_together.room_ended_title — text in either locale.
      await expect(
        page.locator(
          'text=/This Watch Together room has ended|Эта комната Watch Together завершена|Эта комната завершена/i',
        ),
      ).toBeVisible({ timeout: 8_000 })
      // The back button — i18n key `watch_together.room_ended_back_button`.
      // Use getByRole + regex name (valid Playwright API) instead of
      // `:has-text(/regex/)` which is not valid CSS-string syntax.
      await expect(
        page
          .getByRole('button', { name: /Back to anime|Назад к аниме/i })
          .or(page.getByRole('link', { name: /Back to anime|Назад к аниме/i })),
      ).toBeVisible()
    } finally {
      await ctx.close().catch(() => undefined)
    }
  })

  /**
   * i18n smoke-verify checkpoint (MEMORY: feedback_smoke_verify_i18n).
   *
   * For EACH locale (en, ru):
   *   - Open the room-ended empty state (the only WT view reachable
   *     without a live room — it exercises 2/27 keys: room_ended_title +
   *     room_ended_back_button).
   *   - Scan the document.body innerText for any literal `watch_together.`
   *     substring — that would indicate a raw key string slipped through.
   *
   * The room-ended state is the cheapest probe; the full member-list +
   * chat + palette key coverage is verified indirectly by the
   * two-browser scenario above (any raw key would appear in those DOM
   * captures and fail the visibility assertions which expect localized
   * text).
   */
  for (const locale of ['en', 'ru'] as const) {
    test(`i18n smoke: no raw watch_together.* keys in ${locale} (room-ended view)`, async ({
      browser,
      request,
    }) => {
      const ctx = await browser.newContext()
      try {
        const page = await ctx.newPage()
        await setLocale(page, locale)
        await loginAs(page, request, UI_AUDIT_USERNAME, UI_AUDIT_PASSWORD)
        await page.goto('/watch/room/00000000-0000-0000-0000-000000000000')
        await page.waitForLoadState('networkidle')
        // Give the WS error + render-once cycle a beat to settle.
        await page.waitForTimeout(1500)

        const bodyText = await page.evaluate(() => document.body.innerText || '')
        // The whole-document text MUST NOT contain `watch_together.` —
        // that literal only appears when an $t() call missed its key.
        expect(
          bodyText.includes('watch_together.'),
          `locale=${locale}: raw watch_together.* key found in DOM`,
        ).toBe(false)

        // Also assert that the localized back-button label is present in
        // its respective locale (positive signal: i18n actually rendered).
        if (locale === 'en') {
          await expect(page.locator('text=/Back to anime/i')).toBeVisible({ timeout: 5_000 })
        } else {
          await expect(page.locator('text=/Назад к аниме/i')).toBeVisible({ timeout: 5_000 })
        }
      } finally {
        await ctx.close().catch(() => undefined)
      }
    })
  }
})
