import { test, expect } from '@playwright/test'

test.describe('HiAnime Player Detailed Debug', () => {
  const testAnimeUrl = '/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25'

  test('trace all network requests during HiAnime playback', async ({ page }) => {
    // Track ALL network requests
    const allRequests: { url: string; status: number; method: string; responseBody?: string }[] = []

    page.on('response', async response => {
      const req = {
        url: response.url(),
        status: response.status(),
        method: response.request().method()
      }

      // Capture response body for API calls
      if (response.url().includes('/api/') && response.status() < 500) {
        try {
          const body = await response.text()
          allRequests.push({ ...req, responseBody: body.substring(0, 1000) })
        } catch {
          allRequests.push(req)
        }
      } else {
        allRequests.push(req)
      }
    })

    // Track console errors
    const consoleErrors: string[] = []
    page.on('console', msg => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text())
      }
    })

    // Track JS exceptions
    const jsErrors: string[] = []
    page.on('pageerror', error => {
      jsErrors.push(error.message)
    })

    console.log('Navigating to anime page...')
    await page.goto(testAnimeUrl)
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(3000)

    // Take screenshot of initial state
    await page.screenshot({ path: 'test-results/1-initial-state.png', fullPage: true })

    // Look for HiAnime tab/button
    console.log('Looking for HiAnime tab...')
    const hiAnimeTab = page.locator('button').filter({ hasText: /hianime/i })
    const tabCount = await hiAnimeTab.count()
    console.log(`Found ${tabCount} HiAnime tabs/buttons`)

    if (tabCount > 0) {
      console.log('Clicking HiAnime tab...')
      await hiAnimeTab.first().click()
      await page.waitForTimeout(3000)
      await page.screenshot({ path: 'test-results/2-after-hianime-click.png', fullPage: true })
    }

    // Check what's visible in the HiAnime player area
    const hiAnimePlayer = page.locator('.hianime-player')
    const playerVisible = await hiAnimePlayer.isVisible()
    console.log(`HiAnime player container visible: ${playerVisible}`)

    if (playerVisible) {
      // Check for loading state
      const loading = page.locator('.hianime-player .animate-spin')
      const isLoading = await loading.isVisible()
      console.log(`Loading spinner visible: ${isLoading}`)

      // Check for "no episodes" message
      const noEpisodes = page.locator('text=Серии не найдены')
      const hasNoEpisodes = await noEpisodes.isVisible()
      console.log(`No episodes message visible: ${hasNoEpisodes}`)

      // Check for episode buttons (within hianime-player)
      const episodeContainer = page.locator('.hianime-player .flex-wrap')
      const containerVisible = await episodeContainer.isVisible()
      console.log(`Episode container visible: ${containerVisible}`)

      // Get all buttons in player
      const allButtons = page.locator('.hianime-player button')
      const buttonCount = await allButtons.count()
      console.log(`Total buttons in HiAnime player: ${buttonCount}`)

      for (let i = 0; i < Math.min(buttonCount, 10); i++) {
        const btn = allButtons.nth(i)
        const text = await btn.textContent()
        console.log(`  Button ${i}: "${text}"`)
      }

      // Look for error messages
      const errorDiv = page.locator('.hianime-player').locator('.bg-pink-500\\/20, [class*="error"]')
      const hasError = await errorDiv.isVisible()
      console.log(`Error div visible: ${hasError}`)

      if (hasError) {
        const errorText = await errorDiv.textContent()
        console.log(`Error text: ${errorText}`)
      }
    }

    // Wait for any async operations
    await page.waitForTimeout(5000)
    await page.screenshot({ path: 'test-results/3-final-state.png', fullPage: true })

    // Print all API requests
    console.log('\n=== API Requests ===')
    allRequests
      .filter(r => r.url.includes('/api/'))
      .forEach(r => {
        console.log(`${r.method} ${r.status} ${r.url}`)
        if (r.responseBody) {
          console.log(`  Response: ${r.responseBody.substring(0, 200)}...`)
        }
      })

    // Print HLS proxy requests specifically
    console.log('\n=== HLS Proxy Requests ===')
    const hlsRequests = allRequests.filter(r => r.url.includes('hls-proxy'))
    console.log(`Found ${hlsRequests.length} hls-proxy requests`)
    hlsRequests.forEach(r => {
      console.log(`${r.method} ${r.status} ${r.url}`)
    })

    // Print console errors
    console.log('\n=== Console Errors ===')
    consoleErrors.forEach(e => console.log(e))

    // Print JS exceptions
    console.log('\n=== JS Exceptions ===')
    jsErrors.forEach(e => console.log(e))

    // Print any failed requests
    console.log('\n=== Failed Requests ===')
    allRequests.filter(r => r.status >= 400).forEach(r => {
      console.log(`${r.method} ${r.status} ${r.url}`)
    })
  })

  test('manually trigger HiAnime episode selection', async ({ page }) => {
    console.log('Navigating to anime page...')
    await page.goto(testAnimeUrl)
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000)

    // Click HiAnime tab
    const hiAnimeTab = page.locator('button').filter({ hasText: /hianime/i })
    if (await hiAnimeTab.isVisible()) {
      await hiAnimeTab.click()
      await page.waitForTimeout(3000)
    }

    // Wait for episodes to load
    console.log('Waiting for episodes...')
    await page.waitForTimeout(5000)

    // Try clicking an episode number button
    const episodeBtn = page.locator('.hianime-player button').filter({ hasText: '1' }).first()
    if (await episodeBtn.isVisible()) {
      console.log('Found episode 1 button, clicking...')
      await episodeBtn.click()
      await page.waitForTimeout(3000)
    } else {
      console.log('Episode 1 button not found')

      // Print what buttons ARE visible
      const allBtns = page.locator('.hianime-player button')
      const count = await allBtns.count()
      console.log(`Found ${count} buttons total`)
      for (let i = 0; i < count; i++) {
        const text = await allBtns.nth(i).textContent()
        console.log(`  Button: "${text}"`)
      }
    }

    // Check for video element
    const video = page.locator('video')
    const hasVideo = await video.isVisible()
    console.log(`Video element visible: ${hasVideo}`)

    if (hasVideo) {
      // Check video source
      const src = await video.getAttribute('src')
      console.log(`Video src: ${src}`)
    }

    await page.screenshot({ path: 'test-results/4-manual-episode.png', fullPage: true })
  })
})
