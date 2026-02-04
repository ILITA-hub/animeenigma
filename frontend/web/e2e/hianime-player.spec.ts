import { test, expect } from '@playwright/test'

test.describe('HiAnime Player Debug', () => {
  // Test the specific anime from user's report
  const testAnimeUrl = '/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25'

  test('should load HiAnime player and play video', async ({ page }) => {
    // Collect console logs
    const consoleLogs: string[] = []
    page.on('console', msg => {
      consoleLogs.push(`[${msg.type()}] ${msg.text()}`)
    })

    // Collect network errors
    const networkErrors: string[] = []
    page.on('requestfailed', request => {
      networkErrors.push(`FAILED: ${request.url()} - ${request.failure()?.errorText}`)
    })

    // Collect all network requests
    const networkRequests: { url: string; status: number; method: string }[] = []
    page.on('response', response => {
      networkRequests.push({
        url: response.url(),
        status: response.status(),
        method: response.request().method()
      })
    })

    // Navigate to anime page
    await page.goto(testAnimeUrl)
    await page.waitForLoadState('networkidle')

    // Wait for page to fully load
    await page.waitForTimeout(2000)

    // Find and click HiAnime tab (look for HiAnime button/tab)
    const hiAnimeTab = page.locator('button, [role="tab"]').filter({ hasText: /hianime/i })
    if (await hiAnimeTab.isVisible()) {
      console.log('Found HiAnime tab, clicking...')
      await hiAnimeTab.click()
      await page.waitForTimeout(2000)
    } else {
      console.log('HiAnime tab not found, checking if already selected')
    }

    // Wait for episodes to load
    await page.waitForTimeout(3000)

    // Check for episode buttons
    const episodeButtons = page.locator('.hianime-player button').filter({ hasText: /^\d+$/ })
    const episodeCount = await episodeButtons.count()
    console.log(`Found ${episodeCount} episode buttons`)

    if (episodeCount > 0) {
      // Click first episode
      console.log('Clicking first episode...')
      await episodeButtons.first().click()
      await page.waitForTimeout(2000)
    }

    // Wait for server selection
    const serverButtons = page.locator('.hianime-player button').filter({ hasText: /hd-1|hd-2|vidcloud/i })
    const serverCount = await serverButtons.count()
    console.log(`Found ${serverCount} server buttons`)

    if (serverCount > 0) {
      console.log('Clicking first server...')
      await serverButtons.first().click()
    }

    // Wait for video to load
    await page.waitForTimeout(5000)

    // Check for video element
    const videoElement = page.locator('video')
    const hasVideo = await videoElement.isVisible()
    console.log(`Video element visible: ${hasVideo}`)

    // Check for error message
    const errorMessage = page.locator('text=Ошибка воспроизведения видео')
    const hasError = await errorMessage.isVisible()
    console.log(`Error message visible: ${hasError}`)

    // Print collected data
    console.log('\n=== Console Logs ===')
    consoleLogs.forEach(log => console.log(log))

    console.log('\n=== Network Errors ===')
    networkErrors.forEach(err => console.log(err))

    console.log('\n=== HLS Proxy Requests ===')
    networkRequests
      .filter(r => r.url.includes('hls-proxy') || r.url.includes('streaming'))
      .forEach(r => console.log(`${r.method} ${r.status} ${r.url}`))

    console.log('\n=== All Failed Requests ===')
    networkRequests
      .filter(r => r.status >= 400)
      .forEach(r => console.log(`${r.method} ${r.status} ${r.url}`))

    // Assertions
    expect(hasError).toBe(false)
  })

  test('debug: check HLS proxy endpoint directly', async ({ page, request }) => {
    // First get a stream URL from the API
    const episodesResponse = await request.get('/api/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25/hianime/episodes')
    const episodesData = await episodesResponse.json()
    console.log('Episodes:', JSON.stringify(episodesData, null, 2))

    if (episodesData.success && episodesData.data.length > 0) {
      const episodeId = episodesData.data[0].id

      // Get servers
      const serversResponse = await request.get(`/api/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25/hianime/servers?episode=${encodeURIComponent(episodeId)}`)
      const serversData = await serversResponse.json()
      console.log('Servers:', JSON.stringify(serversData, null, 2))

      if (serversData.success && serversData.data.length > 0) {
        const serverId = serversData.data[0].id

        // Get stream
        const streamResponse = await request.get(`/api/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25/hianime/stream?episode=${encodeURIComponent(episodeId)}&server=${serverId}&category=sub`)
        const streamData = await streamResponse.json()
        console.log('Stream:', JSON.stringify(streamData, null, 2))

        if (streamData.success && streamData.data.url) {
          const hlsUrl = streamData.data.url
          const referer = streamData.data.headers?.Referer || ''

          console.log(`\nHLS URL: ${hlsUrl}`)
          console.log(`Referer: ${referer}`)

          // Test proxy endpoint
          const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(hlsUrl)}&referer=${encodeURIComponent(referer)}`
          console.log(`\nProxy URL: ${proxyUrl}`)

          const proxyResponse = await request.get(proxyUrl)
          console.log(`Proxy response status: ${proxyResponse.status()}`)
          console.log(`Proxy response headers: ${JSON.stringify(proxyResponse.headers(), null, 2)}`)

          if (proxyResponse.status() !== 200) {
            const body = await proxyResponse.text()
            console.log(`Proxy error body: ${body}`)
          } else {
            const body = await proxyResponse.text()
            console.log(`Proxy response (first 500 chars): ${body.substring(0, 500)}`)
          }

          expect(proxyResponse.status()).toBe(200)
        }
      }
    }
  })

  test('debug: test proxy status endpoint', async ({ request }) => {
    const response = await request.get('/api/streaming/proxy-status')
    const data = await response.json()
    console.log('Proxy status:', JSON.stringify(data, null, 2))
    expect(response.status()).toBe(200)
  })
})
