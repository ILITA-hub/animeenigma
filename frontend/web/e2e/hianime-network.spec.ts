import { test, expect } from '@playwright/test'

test('capture all network during HiAnime playback', async ({ page }) => {
  const testAnimeUrl = '/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25'

  // Track network
  const requests: { url: string; status?: number; error?: string }[] = []

  page.on('request', req => {
    requests.push({ url: req.url() })
  })

  page.on('response', resp => {
    const idx = requests.findIndex(r => r.url === resp.url() && !r.status)
    if (idx >= 0) {
      requests[idx].status = resp.status()
    }
  })

  page.on('requestfailed', req => {
    const idx = requests.findIndex(r => r.url === req.url() && !r.status)
    if (idx >= 0) {
      requests[idx].error = req.failure()?.errorText || 'unknown'
    }
  })

  // Track console
  const logs: string[] = []
  page.on('console', msg => logs.push(`[${msg.type()}] ${msg.text()}`))

  // Navigate
  await page.goto(testAnimeUrl)
  await page.waitForTimeout(2000)

  // Click HiAnime
  const hiAnimeTab = page.locator('button').filter({ hasText: /hianime/i })
  if (await hiAnimeTab.isVisible()) {
    await hiAnimeTab.click()
    await page.waitForTimeout(3000)
  }

  // Click episode 1
  const ep1 = page.locator('.hianime-player button').filter({ hasText: /^1$/ }).first()
  if (await ep1.isVisible()) {
    await ep1.click()
    await page.waitForTimeout(2000)
  }

  // Click HD-1 server
  const server = page.locator('.hianime-player button').filter({ hasText: /HD-1/i }).first()
  if (await server.isVisible()) {
    await server.click()
    await page.waitForTimeout(5000)
  }

  // Print results
  console.log('\n=== HiAnime/Streaming API Requests ===')
  requests
    .filter(r => r.url.includes('hianime') || r.url.includes('streaming') || r.url.includes('hls-proxy'))
    .forEach(r => console.log(`${r.status || r.error || 'pending'} ${r.url}`))

  console.log('\n=== Console Errors ===')
  logs.filter(l => l.includes('[error]')).forEach(l => console.log(l))

  console.log('\n=== HiAnime Debug Logs ===')
  logs.filter(l => l.includes('[HiAnime]')).forEach(l => console.log(l))

  console.log('\n=== HLS/Video Requests ===')
  requests
    .filter(r => r.url.includes('.m3u8') || r.url.includes('.ts') || r.url.includes('hls'))
    .forEach(r => console.log(`${r.status || r.error || 'pending'} ${r.url}`))

  // Check video state
  const video = page.locator('video')
  const src = await video.getAttribute('src')
  const readyState = await video.evaluate((v: HTMLVideoElement) => v.readyState)
  const error = await video.evaluate((v: HTMLVideoElement) => v.error?.message || 'none')

  console.log(`\n=== Video State ===`)
  console.log(`src: ${src}`)
  console.log(`readyState: ${readyState}`)
  console.log(`error: ${error}`)

  // Check for error message in UI
  const errorMsg = page.locator('text=Ошибка воспроизведения видео')
  const hasError = await errorMsg.isVisible()
  console.log(`UI error visible: ${hasError}`)

  await page.screenshot({ path: 'test-results/network-test.png', fullPage: true })
})
