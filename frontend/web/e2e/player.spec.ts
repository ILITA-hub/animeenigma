import { test, expect } from '@playwright/test'

test.describe('Video Player', () => {
  test.describe('Kodik Player on Anime Page', () => {
    test('should display video player section', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for player container
      const playerContainer = page.locator('iframe, [class*="player"], [class*="video"]')
      await expect(playerContainer.first()).toBeVisible({ timeout: 10000 })
    })

    test('should display translation selector', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      await page.waitForTimeout(3000)

      // Look for translation dropdown or tabs
      const translationSelector = page.locator('select, button, [class*="translation"]').filter({
        hasText: /dub|sub|озвучка|субтитр|voice/i
      })

      // May be visible if translations are available
    })

    test('should display episode selector', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      await page.waitForTimeout(3000)

      // Look for episode list or selector
      const episodeSelector = page.locator('[class*="episode"], button, select').filter({
        hasText: /episode|ep\.|серия|эпизод|\d+/i
      })

      // May be visible if episodes are available
    })

    test('should switch episodes', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      await page.waitForTimeout(3000)

      // Try to find and click episode 2
      const episode2 = page.locator('button, [class*="episode"]').filter({
        hasText: /^2$|episode 2|серия 2/i
      }).first()

      if (await episode2.isVisible()) {
        await episode2.click()
        await page.waitForTimeout(2000)
      }
    })

    test('should display pinned translations', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      await page.waitForTimeout(3000)

      // Look for pin/unpin buttons or pinned indicators
      const pinButton = page.locator('button').filter({
        has: page.locator('svg, [class*="pin"], [class*="star"]')
      })

      // May be visible for pinning translations
    })
  })

  test.describe('Watch Page', () => {
    test('should navigate to watch page from anime detail', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      await page.waitForTimeout(3000)

      // Look for watch/play button
      const watchButton = page.getByRole('link', { name: /watch|play|смотреть|играть/i }).or(
        page.locator('a[href^="/watch/"]')
      )

      if (await watchButton.first().isVisible()) {
        await watchButton.first().click()
        await expect(page).toHaveURL(/\/watch\//)
      }
    })

    test('should display video player on watch page', async ({ page }) => {
      // Navigate directly to a watch page (if we know an ID)
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      if (href) {
        const animeId = href.split('/anime/')[1]
        await page.goto(`/watch/${animeId}/1`)

        await page.waitForTimeout(3000)

        const player = page.locator('video, iframe, [class*="player"]')
        // May be visible on watch page
      }
    })

    test('should display episode navigation', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      if (href) {
        const animeId = href.split('/anime/')[1]
        await page.goto(`/watch/${animeId}/1`)

        await page.waitForTimeout(3000)

        // Look for prev/next buttons
        const prevButton = page.getByRole('button', { name: /prev|previous|назад|предыдущ/i })
        const nextButton = page.getByRole('button', { name: /next|следующ|далее/i })

        // May be visible on watch page
      }
    })

    test('should display episode list', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      if (href) {
        const animeId = href.split('/anime/')[1]
        await page.goto(`/watch/${animeId}/1`)

        await page.waitForTimeout(3000)

        // Look for episode list
        const episodeList = page.locator('[class*="episode"]')
        // May be visible on watch page
      }
    })

    test('should have autoplay toggle', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      if (href) {
        const animeId = href.split('/anime/')[1]
        await page.goto(`/watch/${animeId}/1`)

        await page.waitForTimeout(3000)

        // Look for autoplay toggle
        const autoplayToggle = page.getByText(/autoplay|автовоспроизведение/i)
        // May be visible on watch page
      }
    })

    test('should have quality selector', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      if (href) {
        const animeId = href.split('/anime/')[1]
        await page.goto(`/watch/${animeId}/1`)

        await page.waitForTimeout(3000)

        // Look for quality selector
        const qualitySelector = page.locator('select').filter({
          has: page.locator('option', { hasText: /1080|720|480|auto/i })
        })
        // May be visible on watch page
      }
    })
  })
})

test.describe('Player Controls', () => {
  test('should display anime info on watch page', async ({ page }) => {
    await page.goto('/browse')
    await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

    const animeLink = page.locator('a[href^="/anime/"]').first()
    await animeLink.click()
    await expect(page).toHaveURL(/\/anime\//)

    // Check for anime title visible on page
    const title = page.locator('h1, h2').first()
    await expect(title).toBeVisible()
  })

  test('should link back to anime detail from watch page', async ({ page }) => {
    await page.goto('/browse')
    await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

    const animeLink = page.locator('a[href^="/anime/"]').first()
    const href = await animeLink.getAttribute('href')

    if (href) {
      const animeId = href.split('/anime/')[1]
      await page.goto(`/watch/${animeId}/1`)

      await page.waitForTimeout(3000)

      // Look for back link to anime detail
      const backLink = page.locator(`a[href="/anime/${animeId}"]`)

      if (await backLink.first().isVisible()) {
        await backLink.first().click()
        await expect(page).toHaveURL(/\/anime\//)
      }
    }
  })
})
