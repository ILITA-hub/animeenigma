import { test, expect } from '@playwright/test'

test('HLS diagnostic - analyze network and player state', async ({ page }) => {
  // Collect network requests
  const requests: { url: string; status: number; contentType: string; size: number }[] = []

  page.on('response', async (response) => {
    const url = response.url()
    if (url.includes('hls-proxy') || url.includes('.m3u8') || url.includes('.ts')) {
      requests.push({
        url: url.substring(0, 100),
        status: response.status(),
        contentType: response.headers()['content-type'] || 'unknown',
        size: parseInt(response.headers()['content-length'] || '0')
      })
    }
  })

  // Collect console errors
  const consoleErrors: string[] = []
  page.on('console', msg => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text())
    }
  })

  // Go to anime page
  await page.goto('/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25')
  await page.waitForLoadState('networkidle')

  // Click Consumet tab
  await page.click('button:has-text("Consumet")')
  await page.waitForTimeout(2000)

  // Wait for video element
  const video = page.locator('video')
  await video.waitFor({ timeout: 30000 })

  // Wait for data to load
  await page.waitForTimeout(10000)

  // Print network requests
  console.log('\n=== Network Requests ===')
  for (const req of requests) {
    console.log(`[${req.status}] ${req.contentType} - ${req.size} bytes - ${req.url}`)
  }

  // Print console errors
  console.log('\n=== Console Errors ===')
  for (const err of consoleErrors) {
    console.log(err)
  }

  // Get detailed video state
  const videoState = await video.evaluate((v: HTMLVideoElement) => {
    return {
      duration: v.duration,
      currentTime: v.currentTime,
      paused: v.paused,
      readyState: v.readyState,
      networkState: v.networkState,
      error: v.error ? { code: v.error.code, message: v.error.message } : null,
      videoWidth: v.videoWidth,
      videoHeight: v.videoHeight,
      buffered: Array.from({ length: v.buffered.length }, (_, i) => ({
        start: v.buffered.start(i),
        end: v.buffered.end(i)
      })),
      src: v.src,
      currentSrc: v.currentSrc,
    }
  })

  console.log('\n=== Video State ===')
  console.log(JSON.stringify(videoState, null, 2))

  // Check if HLS.js is attached and its state
  const hlsState = await page.evaluate(() => {
    // @ts-ignore
    if (typeof Hls !== 'undefined') {
      return {
        isSupported: Hls.isSupported(),
        // Check if there's an HLS instance attached to video
      }
    }
    return null
  })

  console.log('\n=== HLS.js State ===')
  console.log(JSON.stringify(hlsState, null, 2))

  // Verify that we got some responses
  expect(requests.length).toBeGreaterThan(0)
})
