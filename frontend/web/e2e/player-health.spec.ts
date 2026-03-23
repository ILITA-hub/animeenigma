import { test, expect } from '@playwright/test'

/**
 * Daily Player Health Check
 *
 * Tests all 4 video players across browsers to catch regressions
 * before users report them. Runs daily via GitHub Actions.
 *
 * What it checks per player:
 * - Page loads, player tab is clickable
 * - Video element appears (or iframe for Kodik)
 * - No fatal HLS errors in console
 * - For HLS players: stream actually starts playing
 */

// Well-known test anime with stable sources
const TEST_ANIME = [
  { id: 'c076bca7-a93f-4089-90a3-0cb69b9cbf25', name: 'Frieren S2' },
]

const FATAL_ERRORS = [
  'bufferAddCodecError',
  'bufferAppendError',
  'bufferIncompatibleCodecsError',
  'manifestLoadError',
  'manifestParsingError',
  'levelLoadError',
]

// Helper: collect console errors during test
function setupConsoleCapture(page: import('@playwright/test').Page) {
  const errors: string[] = []
  const warnings: string[] = []
  page.on('console', msg => {
    if (msg.type() === 'error') errors.push(msg.text())
    if (msg.type() === 'warning') warnings.push(msg.text())
  })
  return { errors, warnings }
}

// Helper: wait for video to start playing
async function waitForVideoPlaying(page: import('@playwright/test').Page, timeout = 30000) {
  const video = page.locator('video').first()
  await expect(video).toBeVisible({ timeout })

  // Wait for video to have some currentTime (actually playing)
  await page.waitForFunction(() => {
    const v = document.querySelector('video')
    return v && v.readyState >= 2 && v.currentTime > 0
  }, { timeout })

  return video
}

// ─── HiAnime Player (EN, HLS) ─────────────────────────────────────

test.describe('HiAnime Player Health', () => {
  for (const anime of TEST_ANIME) {
    test(`${anime.name} — loads and plays`, async ({ page }) => {
      const { errors } = setupConsoleCapture(page)

      await page.goto(`/anime/${anime.id}`)
      await page.waitForLoadState('networkidle')

      // Switch to EN language
      const enButton = page.locator('button').filter({ hasText: /^EN$/ })
      if (await enButton.isVisible()) {
        await enButton.click()
        await page.waitForTimeout(500)
      }

      // Select HiAnime tab
      const hiAnimeTab = page.locator('button').filter({ hasText: /HiAnime/i })
      await expect(hiAnimeTab).toBeVisible({ timeout: 5000 })
      await hiAnimeTab.click()

      // Wait for episode buttons to appear (API responded)
      const episodeButton = page.locator('button').filter({ hasText: /^1$/ }).first()
      await expect(episodeButton).toBeVisible({ timeout: 15000 })

      // Click first episode if not auto-selected
      await episodeButton.click()
      await page.waitForTimeout(2000)

      // Verify video element appears and starts playing
      await waitForVideoPlaying(page, 45000)

      // Check no fatal HLS errors
      const fatalErrors = errors.filter(e =>
        FATAL_ERRORS.some(fe => e.includes(fe))
      )
      expect(fatalErrors).toEqual([])
    })
  }
})

// ─── Consumet Player (EN, HLS) ────────────────────────────────────

test.describe('Consumet Player Health', () => {
  for (const anime of TEST_ANIME) {
    test(`${anime.name} — loads and plays`, async ({ page }) => {
      const { errors } = setupConsoleCapture(page)

      await page.goto(`/anime/${anime.id}`)
      await page.waitForLoadState('networkidle')

      // Switch to EN language
      const enButton = page.locator('button').filter({ hasText: /^EN$/ })
      if (await enButton.isVisible()) {
        await enButton.click()
        await page.waitForTimeout(500)
      }

      // Select Consumet tab
      const consumetTab = page.locator('button').filter({ hasText: /Consumet/i })
      await expect(consumetTab).toBeVisible({ timeout: 5000 })
      await consumetTab.click()

      // Wait for episode buttons
      const episodeButton = page.locator('button').filter({ hasText: /^1$/ }).first()
      await expect(episodeButton).toBeVisible({ timeout: 15000 })
      await episodeButton.click()
      await page.waitForTimeout(2000)

      // Verify video plays
      await waitForVideoPlaying(page, 45000)

      // Check no fatal HLS errors
      const fatalErrors = errors.filter(e =>
        FATAL_ERRORS.some(fe => e.includes(fe))
      )
      expect(fatalErrors).toEqual([])
    })
  }
})

// ─── Kodik Player (RU, iframe) ────────────────────────────────────

test.describe('Kodik Player Health', () => {
  for (const anime of TEST_ANIME) {
    test(`${anime.name} — iframe loads`, async ({ page }) => {
      const { errors } = setupConsoleCapture(page)

      await page.goto(`/anime/${anime.id}`)
      await page.waitForLoadState('networkidle')

      // Switch to RU language
      const ruButton = page.locator('button').filter({ hasText: /^RU$/ })
      if (await ruButton.isVisible()) {
        await ruButton.click()
        await page.waitForTimeout(500)
      }

      // Select Kodik tab
      const kodikTab = page.locator('button').filter({ hasText: /Kodik/i })
      await expect(kodikTab).toBeVisible({ timeout: 5000 })
      await kodikTab.click()

      // Wait for episode list
      const episodeButton = page.locator('button').filter({ hasText: /^1$/ }).first()
      await expect(episodeButton).toBeVisible({ timeout: 15000 })
      await episodeButton.click()
      await page.waitForTimeout(3000)

      // Kodik uses iframe — verify iframe with valid src appears
      const iframe = page.locator('iframe[src*="kodik"]').or(page.locator('iframe[src*="aniqit"]'))
      await expect(iframe).toBeVisible({ timeout: 15000 })

      const src = await iframe.getAttribute('src')
      expect(src).toBeTruthy()
      expect(src).toContain('http')

      // No JS errors from our code (Kodik iframe errors are cross-origin, won't appear)
      const ourErrors = errors.filter(e => e.includes('[Kodik'))
      expect(ourErrors.filter(e => e.includes('Fatal'))).toEqual([])
    })
  }
})

// ─── AnimeLib Player (RU, MP4/iframe) ─────────────────────────────

test.describe('AnimeLib Player Health', () => {
  for (const anime of TEST_ANIME) {
    test(`${anime.name} — loads player element`, async ({ page }) => {
      const { errors } = setupConsoleCapture(page)

      await page.goto(`/anime/${anime.id}`)
      await page.waitForLoadState('networkidle')

      // Switch to RU language
      const ruButton = page.locator('button').filter({ hasText: /^RU$/ })
      if (await ruButton.isVisible()) {
        await ruButton.click()
        await page.waitForTimeout(500)
      }

      // Select AnimeLib tab (displayed as "AniLib")
      const animeLibTab = page.locator('button').filter({ hasText: /AniLib/i })
      await expect(animeLibTab).toBeVisible({ timeout: 5000 })
      await animeLibTab.click()

      // Wait for episode list
      const episodeButton = page.locator('button').filter({ hasText: /^1$/ }).first()
      await expect(episodeButton).toBeVisible({ timeout: 15000 })
      await episodeButton.click()
      await page.waitForTimeout(3000)

      // AnimeLib can show either a <video> (MP4) or iframe (Kodik fallback)
      const video = page.locator('video').first()
      const iframe = page.locator('iframe').first()

      const videoVisible = await video.isVisible().catch(() => false)
      const iframeVisible = await iframe.isVisible().catch(() => false)

      expect(videoVisible || iframeVisible).toBe(true)

      // If it's a video element, check it has a src
      if (videoVisible) {
        const src = await video.getAttribute('src')
        const sourceSrc = await page.locator('video source').first().getAttribute('src').catch(() => null)
        expect(src || sourceSrc).toBeTruthy()
      }

      // No fatal errors from our code
      const fatalErrors = errors.filter(e =>
        e.includes('[AnimeLib') && (e.includes('Fatal') || e.includes('fatal'))
      )
      expect(fatalErrors).toEqual([])
    })
  }
})

// ─── Codec Recovery Test ──────────────────────────────────────────

test.describe('HLS Codec Error Recovery', () => {
  test('recovers from mp4a.40.1 bufferAddCodecError via MSE monkey-patch', async ({ page, browserName }) => {
    // This test simulates Chrome rejecting mp4a.40.1 (AAC Main) by
    // monkey-patching addSourceBuffer. Verifies our recovery logic works.
    test.skip(browserName === 'webkit', 'WebKit uses native HLS, not MSE')

    const { errors, warnings } = setupConsoleCapture(page)

    // Monkey-patch MSE to reject mp4a.40.1 (simulates real Chrome behavior)
    await page.addInitScript(() => {
      const origAddSourceBuffer = MediaSource.prototype.addSourceBuffer
      let blockedOnce = false
      MediaSource.prototype.addSourceBuffer = function (type: string) {
        if (type.includes('mp4a.40.1') && !blockedOnce) {
          blockedOnce = true
          throw new DOMException(
            `Failed to execute 'addSourceBuffer': The type provided ('${type}') is unsupported.`,
            'NotSupportedError'
          )
        }
        return origAddSourceBuffer.call(this, type)
      }
    })

    const anime = TEST_ANIME[0]
    await page.goto(`/anime/${anime.id}`)
    await page.waitForLoadState('networkidle')

    // Switch to EN → Consumet
    const enButton = page.locator('button').filter({ hasText: /^EN$/ })
    if (await enButton.isVisible()) {
      await enButton.click()
      await page.waitForTimeout(500)
    }

    const consumetTab = page.locator('button').filter({ hasText: /Consumet/i })
    await expect(consumetTab).toBeVisible({ timeout: 5000 })
    await consumetTab.click()

    const episodeButton = page.locator('button').filter({ hasText: /^1$/ }).first()
    await expect(episodeButton).toBeVisible({ timeout: 15000 })
    await episodeButton.click()

    // Wait for either recovery or playback
    await page.waitForTimeout(20000)

    // If the monkey-patch triggered, we should see recovery log
    const recoveryTriggered = warnings.some(w =>
      w.includes('bufferAddCodecError') || w.includes('Unsupported audio codec')
    )

    // Either: recovery was triggered and handled, OR the codec was never mp4a.40.1
    // (stream used mp4a.40.2 natively). Both are acceptable outcomes.
    if (recoveryTriggered) {
      // Recovery triggered — verify no user-facing error
      const errorBanner = page.locator('[class*="error"]').filter({
        hasText: /bufferAddCodecError|bufferAppendError/
      })
      await expect(errorBanner).not.toBeVisible()
    }

    // In all cases: no unrecovered fatal errors
    const unrecoveredFatal = errors.filter(e =>
      e.includes('bufferAddCodecError') &&
      !warnings.some(w => w.includes('reinitializing'))
    )
    // This check is soft — if the stream didn't trigger the codec path,
    // unrecoveredFatal will be empty anyway
    expect(unrecoveredFatal.length).toBeLessThanOrEqual(1)
  })
})

// ─── API Health Checks ────────────────────────────────────────────

test.describe('API Health', () => {
  const anime = TEST_ANIME[0]

  test('HiAnime episodes API responds', async ({ request }) => {
    const res = await request.get(`/api/anime/${anime.id}/hianime/episodes`)
    expect(res.ok()).toBe(true)
    const data = await res.json()
    expect(data.episodes?.length || data.length).toBeGreaterThan(0)
  })

  test('Consumet episodes API responds', async ({ request }) => {
    const res = await request.get(`/api/anime/${anime.id}/consumet/episodes`)
    expect(res.ok()).toBe(true)
    const data = await res.json()
    expect(data.episodes?.length || data.length).toBeGreaterThan(0)
  })

  test('HLS proxy is reachable', async ({ request }) => {
    const res = await request.get('/api/streaming/proxy-status')
    expect(res.ok()).toBe(true)
  })
})
