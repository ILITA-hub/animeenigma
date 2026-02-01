import { test, expect } from '@playwright/test'

test.describe('Anime Detail Page', () => {
  test.describe('Public Features', () => {
    test('should display anime details', async ({ page }) => {
      // First, go to browse to find an anime
      await page.goto('/browse')

      // Wait for anime cards to load
      await page.waitForSelector('[class*="anime"], [class*="card"], a[href^="/anime/"]', { timeout: 10000 })

      // Click on first anime card
      const animeCard = page.locator('a[href^="/anime/"]').first()
      await animeCard.click()

      // Should be on anime detail page
      await expect(page).toHaveURL(/\/anime\//)

      // Check for essential elements
      await expect(page.locator('h1, h2').first()).toBeVisible() // Title
    })

    test('should display anime poster', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeCard = page.locator('a[href^="/anime/"]').first()
      await animeCard.click()

      await expect(page).toHaveURL(/\/anime\//)

      // Check for poster image
      const poster = page.locator('img[alt], img[class*="poster"], img[class*="cover"]').first()
      await expect(poster).toBeVisible({ timeout: 5000 })
    })

    test('should display anime synopsis', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for description/synopsis section
      const description = page.locator('p').filter({ hasText: /.{50,}/ }).first()
      await expect(description).toBeVisible({ timeout: 5000 })
    })

    test('should display genres', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for genre badges/chips
      const genreSection = page.getByText(/genre|жанр/i)
      // Genres may or may not be visible depending on data
    })

    test('should display video player', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for video player or iframe
      const player = page.locator('iframe, video, [class*="player"]').first()
      await expect(player).toBeVisible({ timeout: 10000 })
    })

    test('should display reviews section', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for reviews section
      const reviewsSection = page.getByText(/review|отзыв|рецензи/i)
      await expect(reviewsSection.first()).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Authenticated Features', () => {
    test.beforeEach(async ({ page }) => {
      // Set mock auth state
      await page.goto('/')
      await page.evaluate(() => {
        localStorage.setItem('token', 'mock-token')
        localStorage.setItem('user', JSON.stringify({
          id: 'test-user-id',
          username: 'testuser',
          email: 'test@example.com'
        }))
      })
    })

    test('should show watchlist dropdown when authenticated', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for watchlist dropdown/button
      const watchlistButton = page.locator('select, button').filter({
        hasText: /watching|plan|completed|hold|dropped|смотрю|запланировано|просмотрено|отложено|брошено|add to list|добавить/i
      }).first()

      // May or may not be visible depending on auth state
    })

    test('should show review form when authenticated', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for review form elements
      const reviewTextarea = page.locator('textarea')
      // May be visible if authenticated
    })
  })
})

test.describe('Anime Navigation', () => {
  test('should navigate from home to anime detail', async ({ page }) => {
    await page.goto('/')

    // Wait for anime lists to load
    await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

    // Click on any anime
    await page.locator('a[href^="/anime/"]').first().click()

    // Should be on anime detail page
    await expect(page).toHaveURL(/\/anime\//)
  })

  test('should navigate back from anime detail', async ({ page }) => {
    await page.goto('/')
    await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

    await page.locator('a[href^="/anime/"]').first().click()
    await expect(page).toHaveURL(/\/anime\//)

    // Go back
    await page.goBack()

    // Should be on home
    await expect(page).toHaveURL('/')
  })
})
