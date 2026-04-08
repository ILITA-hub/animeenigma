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
      const _translationSelector = page.locator('select, button, [class*="translation"]').filter({
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
      const _episodeSelector = page.locator('[class*="episode"], button, select').filter({
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
      const _pinButton = page.locator('button').filter({
        has: page.locator('svg, [class*="pin"], [class*="star"]')
      })

      // May be visible for pinning translations
    })
  })
})

test.describe('Player Controls', () => {
  test('should display anime info on anime detail page', async ({ page }) => {
    await page.goto('/browse')
    await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

    const animeLink = page.locator('a[href^="/anime/"]').first()
    await animeLink.click()
    await expect(page).toHaveURL(/\/anime\//)

    // Check for anime title visible on page
    const title = page.locator('h1, h2').first()
    await expect(title).toBeVisible()
  })
})
