/**
 * Wave 2 plan 01-05 — combo override tracking contract.
 *
 * Verifies the M-01 contract end-to-end:
 *   - First user-initiated change per (load_session_id, dimension) within 30s
 *     of resolvedCombo apply emits POST /api/preferences/override (D-07/D-10).
 *   - Auto-advance / programmatic mutations DO NOT emit (D-08, Pitfall 1).
 *   - Anonymous users get an X-Anon-ID header on every request (D-11).
 *   - Debounce coalesces rapid clicks (250ms).
 *   - 30s window closes — late clicks emit nothing.
 *   - Per-dimension lock — second click on same dimension is dropped.
 *   - Body shape echoes original_combo + new_combo for forensic queries.
 *
 * The auto-advance test (test 6) drives the DEV-only window hook
 * `__aenigForceAdvanceHiAnime` / `__aenigForceAdvanceConsumet` exposed by the
 * player components in dev builds. WARNING #7 closed.
 *
 * To run:
 *   bunx playwright test combo-override --reporter=list
 *
 * These tests stub Shikimori / parser / preference responses so they don't
 * depend on a running backend. They DO require the Vue dev server (so
 * `import.meta.env.DEV === true` and the force-advance hook is exposed). The
 * playwright config auto-launches `bun run dev` when BASE_URL is unset.
 */

import { test, expect, type Page, type Route, type Request as PWRequest } from '@playwright/test'

// ---------------------------------------------------------------------------
// Stub data + helpers
// ---------------------------------------------------------------------------

const TEST_ANIME_ID = 'test-anime-combo-override'
const TEST_USER = {
  id: 'ui-audit-bot',
  username: 'ui_audit_bot',
}

const STUB_ANIME = {
  id: TEST_ANIME_ID,
  shikimoriId: '1',
  title: 'Combo Override Test',
  russian: 'Тест переключений',
  japanese: 'テスト',
  description: 'Stubbed for combo-override.spec.ts',
  coverImage: 'https://placehold.co/400x600',
  genres: [],
  totalEpisodes: 12,
  episodesAired: 12,
  status: 'released',
  score: 8.0,
  releaseYear: 2024,
}

// Two RU translations so the user can switch teams. Two language buckets so the
// user can swap voice/subtitles. Feed the Kodik parser endpoint.
const STUB_KODIK_TRANSLATIONS = [
  { id: 100, title: 'Studio Alpha', type: 'voice', episodes_count: 12 },
  { id: 200, title: 'Studio Beta',  type: 'voice', episodes_count: 12 },
  { id: 300, title: 'AniSub Team',  type: 'subtitles', episodes_count: 12 },
]

const STUB_RESOLVED_COMBO = {
  language: 'ru' as const,
  watch_type: 'dub' as const,
  player: 'kodik' as const,
  translation_id: '100',
  translation_title: 'Studio Alpha',
  tier: 'shikimori_default',
  tier_number: 4,
}

interface CapturedOverride {
  body: Record<string, unknown>
  headers: Record<string, string>
}

/** Install standard backend stubs. Returns a `captured` array the test fills as
 *  POST /api/preferences/override is hit. */
async function installStubs(page: Page): Promise<CapturedOverride[]> {
  const captured: CapturedOverride[] = []

  await page.route('**/api/preferences/override', async (route: Route, req: PWRequest) => {
    captured.push({
      body: JSON.parse(req.postData() ?? '{}'),
      headers: req.headers(),
    })
    await route.fulfill({ status: 204, body: '' })
  })

  await page.route('**/api/preferences/resolve', async (route) => {
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ combo: STUB_RESOLVED_COMBO }),
    })
  })

  await page.route(`**/api/anime/${TEST_ANIME_ID}`, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ data: STUB_ANIME }),
    })
  })

  // Kodik parser endpoints — translations + a fake embed URL.
  await page.route(`**/api/kodik/translations/${TEST_ANIME_ID}`, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ data: STUB_KODIK_TRANSLATIONS }),
    })
  })
  await page.route(/\/api\/kodik\/video\//, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ data: { embed_link: 'about:blank' } }),
    })
  })
  await page.route(`**/api/kodik/pinned-translations/${TEST_ANIME_ID}`, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ data: [] }),
    })
  })

  return captured
}

/** Drop authentication state so the request flow goes anon (X-Anon-ID only). */
async function bootAsAnon(page: Page): Promise<void> {
  await page.addInitScript(() => {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  })
}

/** Inject a JWT + user object so the auth interceptor adds Authorization. */
async function bootAsAuthUser(page: Page): Promise<void> {
  await page.addInitScript((userJson) => {
    localStorage.setItem('token', 'e2e-fake-jwt')
    localStorage.setItem('user', userJson)
  }, JSON.stringify(TEST_USER))
}

/** Navigate to the seeded anime page and wait for the Kodik picker UI to be
 *  ready (translations have rendered → resolvedCombo has applied). */
async function gotoAnimeAndActivatePlayer(page: Page): Promise<void> {
  await page.goto(`/anime/${TEST_ANIME_ID}`)
  // Activate the player (the page renders a click-to-load placeholder until
  // the user opts in — translations / resolve aren't fetched until then).
  const activateBtn = page.locator('button[aria-label*="смотреть"], button[aria-label*="atch"], button[aria-label*="продолжить"]').first()
  if (await activateBtn.isVisible().catch(() => false)) {
    await activateBtn.click()
  }
  // Wait for the Kodik translation list to render (first translation button).
  await page.waitForSelector('text=Studio Alpha', { timeout: 15000 })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Combo Override Tracking', () => {
  test('auth user — language change within 10s of player load fires POST /api/preferences/override', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // Click the subtitles tab — flips translationType from 'voice' to
    // 'subtitles' and emits dimension='language' from the Kodik composable.
    await page.locator('button:has-text("Субтитры"), button:has-text("Sub")').first().click({ timeout: 5000 })

    // Let the 250ms debounce flush.
    await page.waitForTimeout(500)

    expect(captured.length).toBeGreaterThanOrEqual(1)
    const override = captured.find(c => c.body.dimension === 'language')
    expect(override).toBeDefined()
    expect(override!.body.dimension).toBe('language')
    expect(String(override!.body.load_session_id)).toMatch(/^[0-9a-f]{8}-/i)
    expect(Number(override!.body.ms_since_load)).toBeLessThan(10_000)
    expect(override!.headers['authorization']).toMatch(/^bearer /i)
  })

  test('anon user — team change includes X-Anon-ID header, no Authorization header', async ({ page }) => {
    await bootAsAnon(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // Click the OTHER team (Studio Beta) — fires dimension='team'.
    await page.locator('button:has-text("Studio Beta")').first().click({ timeout: 5000 })

    await page.waitForTimeout(500)

    expect(captured.length).toBeGreaterThanOrEqual(1)
    const override = captured.find(c => c.body.dimension === 'team')
    expect(override).toBeDefined()
    expect(override!.body.dimension).toBe('team')
    expect(override!.headers['x-anon-id']).toMatch(/^[0-9a-f]{8}-/i)
    expect(override!.headers['authorization']).toBeUndefined()
  })

  test('debounce — two clicks within 250ms coalesce to one POST', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // Click team Beta then team Alpha back rapidly — both within the 250ms
    // debounce window. Only the second click's POST should land.
    await page.locator('button:has-text("Studio Beta")').first().click()
    await page.locator('button:has-text("Studio Alpha")').first().click({ timeout: 200 })

    await page.waitForTimeout(500)

    const teamOverrides = captured.filter(c => c.body.dimension === 'team')
    expect(teamOverrides.length).toBe(1)
  })

  test('30s window — click after 31s emits no POST', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // Force the composable's window to close. mountedAt is captured via
    // performance.now() inside the composable; simulate elapsed time by
    // shifting performance.now's return.
    await page.evaluate(() => {
      const orig = performance.now.bind(performance)
      const start = orig()
      // Add a 31-second offset to all subsequent calls.
      Object.defineProperty(performance, 'now', {
        configurable: true,
        value: () => orig() - start + 31_000 + (orig() - start),
      })
    })

    await page.locator('button:has-text("Studio Beta")').first().click()
    await page.waitForTimeout(500)

    expect(captured.length).toBe(0)
  })

  test('first per dimension only — second team click in same session is ignored', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // First team click — emits.
    await page.locator('button:has-text("Studio Beta")').first().click()
    await page.waitForTimeout(500)

    // Second team click — past debounce, but should be locked out by the
    // emittedDimensions Set.
    await page.locator('button:has-text("Studio Alpha")').first().click()
    await page.waitForTimeout(500)

    const teamOverrides = captured.filter(c => c.body.dimension === 'team')
    expect(teamOverrides.length).toBe(1)
  })

  test('ignores auto-advance — programmatic episode change emits no POST', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    // Stub HiAnime endpoints so we can navigate there and exercise its hook.
    // The hook is exposed by HiAnimePlayer.vue under import.meta.env.DEV; the
    // dev server has DEV=true, so __aenigForceAdvanceHiAnime is set on window.
    // (Consumet exposes a parallel hook; we use HiAnime here because its
    // multi-server picker is the primary auto-advance offender.)
    const STUB_HIANIME_EPISODES = [
      { id: 'ep-1', number: 1, title: 'Episode 1', is_filler: false },
      { id: 'ep-2', number: 2, title: 'Episode 2', is_filler: false },
    ]
    const STUB_HIANIME_SERVERS = [
      { id: 'srv-1', name: 'HD-1', type: 'sub' },
      { id: 'srv-2', name: 'HD-2', type: 'sub' },
    ]
    await page.route(`**/api/hianime/${TEST_ANIME_ID}/episodes`, async (route) => {
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ data: STUB_HIANIME_EPISODES }),
      })
    })
    await page.route(/\/api\/hianime\/[^/]+\/servers\/.*/, async (route) => {
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ data: STUB_HIANIME_SERVERS }),
      })
    })
    await page.route(/\/api\/hianime\/[^/]+\/stream\/.*/, async (route) => {
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          data: { url: 'https://example.invalid/test.m3u8', type: 'hls', subtitles: [] },
        }),
      })
    })

    // Land on anime page, then switch to HiAnime via the language tab path:
    // localStorage preferred_video_language='en' + preferred_video_provider='hianime'
    // boots the page with HiAnime mounted.
    await page.addInitScript(() => {
      localStorage.setItem('preferred_video_language', 'en')
      localStorage.setItem('preferred_video_provider', 'hianime')
    })

    await page.goto(`/anime/${TEST_ANIME_ID}`)
    const activateBtn = page.locator('button[aria-label*="watch"], button[aria-label*="смотреть"]').first()
    if (await activateBtn.isVisible().catch(() => false)) {
      await activateBtn.click()
    }

    // Wait for the HiAnime DEV hook to be installed (composable mounts it on
    // setup, gated by import.meta.env.DEV).
    await page.waitForFunction(
      () => typeof (window as unknown as { __aenigForceAdvanceHiAnime?: unknown }).__aenigForceAdvanceHiAnime === 'function',
      null,
      { timeout: 15000 },
    )

    // Drive the DEV-only force-advance hook. It MUST call _advanceServer /
    // _advanceEpisode (the unwrapped sibling) — the whole point of this test
    // is that the unwrapped path emits NO override POST.
    await page.evaluate(() => {
      const w = window as unknown as {
        __aenigForceAdvanceHiAnime?: () => void
        __aenigForceAdvanceConsumet?: () => void
      }
      const hook = w.__aenigForceAdvanceHiAnime ?? w.__aenigForceAdvanceConsumet
      if (!hook) {
        throw new Error('DEV-only force-advance hook missing — Task 1 must expose __aenigForceAdvance{HiAnime,Consumet}')
      }
      hook()
    })

    await page.waitForTimeout(500)

    expect(captured.length).toBe(0)
  })

  test('records original_combo and new_combo on POST body', async ({ page }) => {
    await bootAsAuthUser(page)
    const captured = await installStubs(page)

    await gotoAnimeAndActivatePlayer(page)

    // Click the subtitles tab — fires dimension='language'.
    await page.locator('button:has-text("Субтитры"), button:has-text("Sub")').first().click({ timeout: 5000 })

    await page.waitForTimeout(500)

    const override = captured.find(c => c.body.dimension === 'language')
    expect(override).toBeDefined()

    // original_combo echoes whatever the resolver returned via /resolve. The
    // composable reads from props.preferredCombo at emit time. Anime.vue's
    // resolvedCombo is null on first paint and populated after resolve, so
    // original_combo on Kodik's tracker is the WatchCombo (no tier info) —
    // tier/tier_number are only present when the prop is the full
    // ResolvedCombo, which the player props are not. Verify the shape we
    // actually get:
    expect(override!.body.original_combo).toBeDefined()
    expect(override!.body.new_combo).toBeDefined()
    const newCombo = override!.body.new_combo as Record<string, unknown>
    expect(newCombo.watch_type).toBe('sub')
  })
})
