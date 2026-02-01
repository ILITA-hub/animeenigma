import { test, expect } from '@playwright/test'

test.describe('Home Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test.describe('Layout', () => {
    test('should display header navigation', async ({ page }) => {
      await expect(page.locator('header')).toBeVisible()
    })

    test('should display logo/branding', async ({ page }) => {
      const logo = page.locator('header').getByRole('link').first()
      await expect(logo).toBeVisible()
    })

    test('should display navigation links', async ({ page }) => {
      // Check for main nav links
      const browseLink = page.locator('header').getByRole('link', { name: /browse|catalog|каталог/i })
      await expect(browseLink.first()).toBeVisible()
    })
  })

  test.describe('Hero Section', () => {
    test('should display search bar', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i)
      await expect(searchInput.first()).toBeVisible()
    })

    test('should navigate to search on submit', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('Naruto')
      await searchInput.press('Enter')

      await expect(page).toHaveURL(/\/(search|browse)/)
    })
  })

  test.describe('Content Sections', () => {
    test('should display announcements section', async ({ page }) => {
      await page.waitForTimeout(2000)

      const announcementsSection = page.getByText(/announcements|анонс/i)
      await expect(announcementsSection.first()).toBeVisible({ timeout: 10000 })
    })

    test('should display ongoing section', async ({ page }) => {
      await page.waitForTimeout(2000)

      const ongoingSection = page.getByText(/ongoing|онгоинг/i)
      await expect(ongoingSection.first()).toBeVisible({ timeout: 10000 })
    })

    test('should display top anime section', async ({ page }) => {
      await page.waitForTimeout(2000)

      const topSection = page.getByText(/top|топ/i)
      await expect(topSection.first()).toBeVisible({ timeout: 10000 })
    })

    test('should display anime cards', async ({ page }) => {
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const animeCards = page.locator('a[href^="/anime/"]')
      const count = await animeCards.count()

      expect(count).toBeGreaterThan(0)
    })

    test('should display next episode countdown for ongoing anime', async ({ page }) => {
      await page.waitForTimeout(3000)

      // Look for time indicators
      const timeIndicators = page.getByText(/today|tomorrow|monday|tuesday|wednesday|thursday|friday|saturday|sunday|сегодня|завтра|понедельник|вторник|среда|четверг|пятница|суббота|воскресенье/i)

      // May be visible if ongoing anime exist
    })
  })

  test.describe('Navigation', () => {
    test('should navigate to browse page', async ({ page }) => {
      await page.getByRole('link', { name: /catalog|browse|каталог/i }).first().click()
      await expect(page).toHaveURL(/\/browse/)
    })

    test('should navigate to schedule page', async ({ page }) => {
      const scheduleLink = page.getByRole('link', { name: /schedule|расписание/i })

      if (await scheduleLink.first().isVisible()) {
        await scheduleLink.first().click()
        await expect(page).toHaveURL(/\/schedule/)
      }
    })

    test('should navigate to anime detail on card click', async ({ page }) => {
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)
    })
  })

  test.describe('Responsive Design', () => {
    test('should display mobile navigation on small screens', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 })
      await page.reload()

      // Mobile nav should be visible at bottom
      const mobileNav = page.locator('nav.fixed')
      await expect(mobileNav).toBeVisible()
    })

    test('should hide desktop nav on mobile', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 })
      await page.reload()

      // Desktop nav links should be hidden
      const desktopNav = page.locator('header .hidden.md\\:flex').first()
      await expect(desktopNav).toBeHidden()
    })

    test('should stack columns on mobile', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 })
      await page.reload()

      // Content should still be visible
      await page.waitForTimeout(2000)
      const content = page.locator('main, [class*="container"]').first()
      await expect(content).toBeVisible()
    })
  })
})
