import { test, expect } from '@playwright/test'

test('verify subtitles are loaded in HiAnime player', async ({ page }) => {
  const testAnimeUrl = '/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25'

  // Track subtitle requests
  const subtitleRequests: { url: string; status?: number }[] = []

  page.on('response', async response => {
    if (response.url().includes('.vtt') || response.url().includes('subtitle')) {
      subtitleRequests.push({
        url: response.url(),
        status: response.status()
      })
    }
  })

  console.log('Navigating to anime page...')
  await page.goto(testAnimeUrl)
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(2000)

  // Click HiAnime tab
  const hiAnimeTab = page.locator('button').filter({ hasText: /hianime/i })
  if (await hiAnimeTab.isVisible()) {
    console.log('Clicking HiAnime tab...')
    await hiAnimeTab.click()
    await page.waitForTimeout(3000)
  }

  // Wait for video to load
  await page.waitForTimeout(5000)

  // Check if video element exists
  const video = page.locator('video')
  const hasVideo = await video.isVisible()
  console.log(`Video element visible: ${hasVideo}`)

  if (hasVideo) {
    // Check for track elements
    const tracks = await video.locator('track').all()
    console.log(`Number of track elements: ${tracks.length}`)

    for (let i = 0; i < tracks.length; i++) {
      const track = tracks[i]
      const src = await track.getAttribute('src')
      const label = await track.getAttribute('label')
      const srclang = await track.getAttribute('srclang')
      const kind = await track.getAttribute('kind')
      const isDefault = await track.getAttribute('default')
      console.log(`Track ${i}: kind=${kind}, label=${label}, srclang=${srclang}, default=${isDefault}`)
      console.log(`  src=${src}`)
    }

    // Check video's text tracks
    const trackInfo = await video.evaluate((v: HTMLVideoElement) => {
      const tracks: { kind: string; label: string; language: string; mode: string }[] = []
      for (let i = 0; i < v.textTracks.length; i++) {
        const t = v.textTracks[i]
        tracks.push({
          kind: t.kind,
          label: t.label,
          language: t.language,
          mode: t.mode
        })
      }
      return tracks
    })

    console.log(`\n=== Video Text Tracks ===`)
    trackInfo.forEach((t, i) => {
      console.log(`Track ${i}: kind=${t.kind}, label=${t.label}, lang=${t.language}, mode=${t.mode}`)
    })

    // Check video state
    const readyState = await video.evaluate((v: HTMLVideoElement) => v.readyState)
    console.log(`\nVideo readyState: ${readyState}`)
  }

  // Print subtitle requests
  console.log(`\n=== Subtitle Requests ===`)
  subtitleRequests.forEach(r => {
    console.log(`${r.status} ${r.url}`)
  })

  // Check subtitles info section in UI
  const subtitlesSection = page.locator('text=Субтитры')
  const hasSubtitlesSection = await subtitlesSection.isVisible()
  console.log(`\nSubtitles UI section visible: ${hasSubtitlesSection}`)

  if (hasSubtitlesSection) {
    const subtitleLabels = await page.locator('.hianime-player').locator('text=Субтитры').locator('..').locator('span').allTextContents()
    console.log(`Subtitle labels in UI: ${subtitleLabels.join(', ')}`)
  }

  await page.screenshot({ path: 'test-results/subtitles-test.png', fullPage: true })
})
