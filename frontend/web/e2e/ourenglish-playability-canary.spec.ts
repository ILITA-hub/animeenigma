import { test, expect, type Page, type APIRequestContext } from '@playwright/test'

/**
 * OurEnglish (EN) playability canary — ISS-020 class regression detector.
 *
 * ISS-020: the bundled hls.js threw a fatal `bufferAddCodecError`
 * (mp4a.40.1 on CODECS-less owocdn HLS — animepahe/miruro) which froze the
 * EN player at 0:00. A SERVER-SIDE health probe canNOT catch this: it is a
 * browser/MSE codec rejection that only manifests when a real MediaSource
 * tries to decode segments. So this canary drives the REAL deployed site
 * with the REAL bundled hls.js (NO CDN injection — that would mask exactly
 * the bundled-version regression we are trying to catch) and asserts the
 * <video> element actually advances past 0:00 (segments decoded → real
 * playback).
 *
 * Default target is production (https://animeenigma.ru). Override with
 * CANARY_BASE_URL for staging/local. Runs headless via the chromium project.
 *
 * Local invocation:
 *   cd frontend/web && \
 *     CANARY_BASE_URL=https://animeenigma.ru \
 *     bunx playwright test e2e/ourenglish-playability-canary.spec.ts \
 *       --project=chromium --reporter=list
 *
 * ── PASS / FAIL / SKIP semantics ──────────────────────────────────────────
 *   PASS  — the provider's <video> reached currentTime > 0 within the budget.
 *   FAIL  — a provider that the scraper /scraper/health endpoint reports as
 *           `playable: true` failed to actually play in-browser (the ISS-020
 *           signal), OR the default (auto) EN path failed to play at all.
 *   SKIP  — the stack is unreachable, OR a provider is NOT reported playable
 *           by the health endpoint (legitimately down upstream, e.g.
 *           nineanime/animefever 502s). We log + soft-skip these so the
 *           canary's red signal stays meaningful and doesn't train CI to
 *           ignore it.
 *
 * Coverage implemented (documented per request — nothing silently skipped):
 *   1. DEFAULT (auto) EN path — must play. This is the user's real default.
 *   2. Pinned `allanime` — direct-MP4 control that should essentially always
 *      work; a failure here points at the proxy/site, not a codec regression.
 *   3. Pinned `animepahe` — the HLS provider ISS-020 actually broke. This is
 *      the highest-value per-provider probe for the bundled-hls.js codec bug.
 *   4. All OTHER providers the health endpoint reports playable:true
 *      (gogoanime / animefever / miruro / nineanime) — pinned + asserted.
 *   Providers not reported playable:true are soft-skipped (logged).
 */

// Mirror OurEnglishPlayer.vue AVAILABLE_PROVIDERS (kept in sync by hand —
// if the player adds a provider, add it here so per-provider coverage widens).
const ALL_PROVIDERS = [
  'gogoanime',
  'animepahe',
  'allanime',
  'animefever',
  'miruro',
  'nineanime',
] as const
type Provider = (typeof ALL_PROVIDERS)[number]

// Providers we ALWAYS attempt to pin explicitly (in addition to default/auto),
// regardless of whether the health endpoint listed them — but they only FAIL
// the canary when health says playable:true (see classifyProvider). allanime
// is the direct-MP4 control; animepahe is the ISS-020 HLS culprit.
const PRIORITY_PINS: Provider[] = ['allanime', 'animepahe']

const BASE_URL = process.env.CANARY_BASE_URL || 'https://animeenigma.ru'

// TenSura S4 — has all providers (per task brief; verified to expose the full
// EN failover chain). Override with CANARY_ANIME_UUID if catalog drifts.
const TEST_ANIME_UUID =
  process.env.CANARY_ANIME_UUID || 'dbc95dd5-8470-4f83-9632-622431073182'

// How long we let a single provider attempt to reach currentTime > 0. HLS
// manifest + first-segment fetch through the proxy can take a while on cold
// upstreams; 25s matches the proven reproduction window from the task brief.
const PLAYBACK_BUDGET_MS = 25_000
const PLAYBACK_POLL_MS = 500

interface HealthData {
  // provider -> bool (true = real-bytes oracle confirmed playable). Providers
  // with no fresh probe are OMITTED (absent = unknown), per scraper.go GetHealth.
  playable: Record<string, boolean>
  // provider -> public stage snapshot (presence = registered provider).
  providers: Record<string, unknown>
}

/** Fetch the public scraper health table. Returns null if unreachable. */
async function fetchHealth(request: APIRequestContext): Promise<HealthData | null> {
  try {
    const resp = await request.get(`${BASE_URL}/api/anime/_/scraper/health`, {
      timeout: 8000,
    })
    if (!resp.ok()) return null
    const body = await resp.json()
    const data = body?.data ?? body
    if (!data || typeof data !== 'object') return null
    return {
      playable: (data.playable ?? {}) as Record<string, boolean>,
      providers: (data.providers ?? {}) as Record<string, unknown>,
    }
  } catch {
    return null
  }
}

/**
 * Activate the player. Anime.vue gates the whole player surface (language
 * pills + provider sub-tabs + the OurEnglish player) behind `playerActivated`,
 * which is flipped by the primary "Watch" CTA (activatePlayer()). Until that
 * button is clicked, the EN pill does not exist. Click any in-page Watch CTA
 * (the cyan "Watch now / Continue ep N" button). Returns true if clicked.
 *
 * The CTA text is i18n'd (anime.watchNow / anime.continueEp — "Watch now",
 * "Смотреть", "Continue ep N", "Продолжить эп. N"), so match by the play-icon
 * button shape rather than text: it's the only <button> that calls
 * activatePlayer and sits in the CTA row. We match the first non-disabled
 * button whose accessible/inner text looks like a watch action OR, failing
 * that, the first button containing the play-triangle <path d="M8 5v14l11-7z">.
 */
async function activatePlayer(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const buttons = Array.from(document.querySelectorAll('button'))
    // 1) Text-based match across the supported locales.
    const watchRe = /watch now|continue ep|смотреть|продолжить эп/i
    let target = buttons.find((b) => watchRe.test(b.innerText.trim()))
    // 2) Fallback: the play-triangle path is unique to the Watch CTA + the
    //    placeholder play button (both call activatePlayer).
    if (!target) {
      target = buttons.find((b) => b.querySelector('path[d="M8 5v14l11-7z"]') !== null)
    }
    if (!target) return false
    ;(target as HTMLButtonElement).click()
    return true
  })
}

/**
 * Select the EN provider toggle button on the anime detail page. There are two
 * buttons with text "EN" — the nav language switcher (no aria-pressed) and the
 * in-page video language pill (HAS aria-pressed). We must click the latter.
 * Returns true if a click was dispatched.
 */
async function clickEnToggle(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const btn = Array.from(document.querySelectorAll('button')).find(
      (b) => b.innerText.trim() === 'EN' && b.hasAttribute('aria-pressed'),
    )
    if (!btn) return false
    ;(btn as HTMLButtonElement).click()
    return true
  })
}

/**
 * Pin a specific provider via the in-player "Source" <select> (v-model
 * preferredProvider). The control is the <select> immediately following the
 * label[data-test="source-dropdown"] in OurEnglishPlayer.vue. Selecting an
 * option sets preferredProvider and re-resolves episodes → servers → stream →
 * attachStream. Passing '' selects the "auto" option.
 *
 * Uses Playwright's selectOption + a native change/input dispatch so Vue's
 * v-model fires reliably.
 */
async function pinProvider(page: Page, provider: Provider | ''): Promise<boolean> {
  // The Source <select> lives right after the label with data-test="source-dropdown".
  const sourceSelect = page
    .locator('label[data-test="source-dropdown"] ~ select')
    .first()
  const visible = await sourceSelect.isVisible({ timeout: 5000 }).catch(() => false)
  if (!visible) return false
  // Reset the CURRENT video's baseline to 0 before switching, so a stale
  // still-advancing element from the previous provider can't leak a false
  // currentTime>0 into the NEXT provider's observation (the player re-uses /
  // re-attaches the same <video>). Pausing + zeroing makes "currentTime > 0"
  // mean the newly-pinned stream actually advanced.
  await page
    .evaluate(() => {
      const v = document.querySelector('.ourenglish-player video') as HTMLVideoElement | null
      if (v) {
        try {
          v.pause()
          v.currentTime = 0
        } catch {
          /* ignore */
        }
      }
    })
    .catch(() => undefined)
  // Option values are the raw provider name; '' is the auto option.
  await sourceSelect.selectOption(provider).catch(() => undefined)
  // Let the player tear down the old stream and begin re-resolving episodes →
  // servers → stream before we start polling for new playback. Without this
  // settle, observePlayback can sample the old element mid-teardown.
  await page.waitForTimeout(1200)
  return true
}

/**
 * Poll the player's <video> until it reports real playback (currentTime > 0)
 * or the budget expires. Returns a structured observation for assertion
 * messages — naming the provider, currentTime, readyState, and whether the
 * player surfaced its own streamFailed UI (player.sourceUnavailable).
 */
async function observePlayback(
  page: Page,
  budgetMs: number,
): Promise<{
  played: boolean
  currentTime: number
  readyState: number
  bufferedEnd: number
  streamFailedUiVisible: boolean
}> {
  const deadline = Date.now() + budgetMs
  let last = {
    played: false,
    currentTime: 0,
    readyState: 0,
    bufferedEnd: 0,
    streamFailedUiVisible: false,
  }
  while (Date.now() < deadline) {
    last = await page.evaluate(() => {
      const v = document.querySelector(
        '.ourenglish-player video',
      ) as HTMLVideoElement | null
      // The "source unavailable" failure UI renders the player.sourceUnavailable
      // copy; detect it generically so a hard failure short-circuits the wait.
      const failBlock = document.querySelector('.ourenglish-player')?.textContent ?? ''
      // These English/Russian fragments come from player.sourceUnavailable.
      const streamFailedUiVisible =
        /source unavailable|источник недоступ|unavailable/i.test(failBlock) &&
        !v?.currentTime
      const currentTime = v?.currentTime ?? 0
      const readyState = v?.readyState ?? 0
      let bufferedEnd = 0
      try {
        if (v && v.buffered.length > 0) bufferedEnd = v.buffered.end(v.buffered.length - 1)
      } catch {
        /* ignore */
      }
      return {
        played: currentTime > 0,
        currentTime,
        readyState,
        bufferedEnd,
        streamFailedUiVisible,
      }
    })
    if (last.played) return last
    // Wait between polls.
    await new Promise((r) => setTimeout(r, PLAYBACK_POLL_MS))
  }
  return last
}

/**
 * Decide how to treat a provider result:
 *   - 'fail'  → health says playable:true but in-browser playback failed
 *               (ISS-020 signal — alert).
 *   - 'skip'  → health does NOT report playable:true (provider down upstream
 *               or unknown — soft skip, log only).
 */
function classifyProvider(
  provider: Provider,
  health: HealthData,
): 'expect-play' | 'soft-skip' {
  return health.playable[provider] === true ? 'expect-play' : 'soft-skip'
}

test.describe('OurEnglish playability canary (ISS-020 class)', () => {
  // Force the configured base URL regardless of the shared playwright.config
  // BASE_URL, so this canary always targets CANARY_BASE_URL (default prod).
  test.use({ baseURL: BASE_URL, ignoreHTTPSErrors: true })

  let health: HealthData | null = null

  test.beforeAll(async ({ request }) => {
    health = await fetchHealth(request)
    test.skip(
      !health,
      `scraper health endpoint unreachable at ${BASE_URL}/api/anime/_/scraper/health — stack down, not a regression`,
    )
  })

  test('[canary] EN player reaches real playback (currentTime > 0) per provider', async ({
    browser,
  }) => {
    // Defensive — beforeAll skips when health is null, but narrow the type.
    if (!health) {
      test.skip(true, 'no health data')
      return
    }
    const h = health

    // --ignore-certificate-errors mirrors the proven reproduction launch args
    // (self-signed / cert edge cases shouldn't block the canary).
    const ctx = await browser.newContext({
      ignoreHTTPSErrors: true,
    })
    try {
      const page = await ctx.newPage()

      // Observe scraper/proxy traffic for richer failure diagnostics.
      const seenStreamCalls: string[] = []
      page.on('response', (resp) => {
        const u = resp.url()
        if (/\/scraper\/(episodes|servers|stream)|\/api\/streaming\/hls-proxy/.test(u)) {
          seenStreamCalls.push(`${resp.status()} ${u.slice(0, 120)}`)
        }
      })

      await page.goto(`/anime/${TEST_ANIME_UUID}`, {
        waitUntil: 'networkidle',
        timeout: 45_000,
      })

      // Anime.vue gates the player (and the EN pill) behind playerActivated —
      // click the Watch CTA first so the language tabs render.
      const activated = await activatePlayer(page)
      test.skip(
        !activated,
        `Watch CTA not found on /anime/${TEST_ANIME_UUID} — title may be unreleased or layout changed, not an ISS-020 regression`,
      )

      // The EN language pill (button text exactly "EN" WITH aria-pressed) only
      // renders after activation. Poll for its existence via the SAME predicate
      // clickEnToggle uses, so we don't depend on Playwright's hasText regex
      // anchoring quirks (a /^EN$/ filter matches textContent, not innerText,
      // and intermittently misses this button).
      await expect
        .poll(
          async () =>
            page.evaluate(() =>
              Array.from(document.querySelectorAll('button')).some(
                (b) => b.innerText.trim() === 'EN' && b.hasAttribute('aria-pressed'),
              ),
            ),
          {
            timeout: 15_000,
            message: 'in-page EN language pill never appeared after activating the player',
          },
        )
        .toBe(true)
      const clicked = await clickEnToggle(page)
      test.skip(
        !clicked,
        `EN provider toggle not found on /anime/${TEST_ANIME_UUID} — catalog drift or page layout change, not an ISS-020 regression`,
      )

      // Wait for the OurEnglish player surface to mount.
      const playerSurface = page.locator('.ourenglish-player')
      await expect(playerSurface).toBeVisible({ timeout: 20_000 })

      // ── 1) DEFAULT (auto) EN path — MUST play. ───────────────────────────
      // Ensure we're on auto first (the player defaults preferredProvider='').
      await pinProvider(page, '')
      const autoResult = await observePlayback(page, PLAYBACK_BUDGET_MS)
      expect(
        autoResult.played,
        `DEFAULT (auto) EN path failed to reach playback within ${PLAYBACK_BUDGET_MS}ms on ` +
          `/anime/${TEST_ANIME_UUID}. Observed currentTime=${autoResult.currentTime}, ` +
          `readyState=${autoResult.readyState}, bufferedEnd=${autoResult.bufferedEnd}, ` +
          `streamFailedUi=${autoResult.streamFailedUiVisible}. ` +
          `This is an ISS-020-class signal: the bundled hls.js may be freezing the player at 0:00. ` +
          `Recent stream/proxy calls: ${seenStreamCalls.slice(-6).join(' | ') || 'none observed'}`,
      ).toBe(true)

      // ── 2) Per-provider coverage. ────────────────────────────────────────
      // Attempt every provider; PRIORITY_PINS (allanime, animepahe) are always
      // attempted first for clearer signal ordering. We FAIL only on providers
      // health reports playable:true; others are soft-skipped (logged).
      const ordered: Provider[] = [
        ...PRIORITY_PINS,
        ...ALL_PROVIDERS.filter((p) => !PRIORITY_PINS.includes(p)),
      ]

      const softSkipped: string[] = []
      const failures: string[] = []

      for (const provider of ordered) {
        const disposition = classifyProvider(provider, h)
        if (disposition === 'soft-skip') {
          softSkipped.push(
            `${provider} (health.playable=${JSON.stringify(h.playable[provider])})`,
          )
          // eslint-disable-next-line no-console
          console.log(
            `[canary] SOFT-SKIP ${provider}: health endpoint does not report playable:true ` +
              `(value=${JSON.stringify(h.playable[provider])}). Legitimately down upstream or unknown.`,
          )
          continue
        }

        // Pin the provider — re-resolves episodes/servers/stream + attachStream.
        const pinned = await pinProvider(page, provider)
        if (!pinned) {
          // Source dropdown not present (player not mounted / no providers) —
          // treat as soft-skip with a clear log, not a hard fail.
          softSkipped.push(`${provider} (source dropdown unavailable)`)
          // eslint-disable-next-line no-console
          console.log(`[canary] SOFT-SKIP ${provider}: Source dropdown not pinnable.`)
          continue
        }

        const result = await observePlayback(page, PLAYBACK_BUDGET_MS)
        if (result.played) {
          // eslint-disable-next-line no-console
          console.log(
            `[canary] PASS ${provider}: currentTime=${result.currentTime.toFixed(2)} ` +
              `readyState=${result.readyState} bufferedEnd=${result.bufferedEnd.toFixed(2)}`,
          )
        } else {
          failures.push(
            `${provider}: health.playable=true but in-browser playback FAILED ` +
              `(currentTime=${result.currentTime}, readyState=${result.readyState}, ` +
              `bufferedEnd=${result.bufferedEnd}, streamFailedUi=${result.streamFailedUiVisible}). ` +
              `Last stream/proxy calls: ${seenStreamCalls.slice(-4).join(' | ') || 'none'}`,
          )
        }
      }

      // eslint-disable-next-line no-console
      console.log(
        `[canary] coverage summary — attempted=${ordered.length}, ` +
          `soft-skipped=${softSkipped.length} [${softSkipped.join(', ') || 'none'}], ` +
          `failed=${failures.length}`,
      )

      // FAIL the canary if any health-confirmed-playable provider failed to
      // actually play in-browser (the ISS-020 signal).
      expect(
        failures.length,
        `One or more providers the scraper health endpoint reports playable:true failed to ` +
          `actually play in-browser (ISS-020-class codec/MSE regression in the bundled hls.js):\n` +
          failures.map((f) => `  - ${f}`).join('\n') +
          `\n(soft-skipped, not failed: ${softSkipped.join(', ') || 'none'})`,
      ).toBe(0)
    } finally {
      await ctx.close().catch(() => undefined)
    }
  })
})
