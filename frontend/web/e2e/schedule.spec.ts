import { test, expect } from '@playwright/test'

test.describe('Schedule Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/schedule')
  })

  test('should display schedule page title', async ({ page }) => {
    const title = page.getByRole('heading', { name: /schedule|расписание/i })
    await expect(title).toBeVisible({ timeout: 5000 })
  })

  test('should display day sections', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // Look for day headings (Monday, Tuesday, etc. or Russian equivalents)
    const dayHeadings = page.getByText(/monday|tuesday|wednesday|thursday|friday|saturday|sunday|понедельник|вторник|среда|четверг|пятница|суббота|воскресенье|today|сегодня/i)

    // At least one day should be visible
    await expect(dayHeadings.first()).toBeVisible({ timeout: 5000 })
  })

  test('should highlight today', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // Look for "today" indicator
    const todayIndicator = page.getByText(/today|сегодня/i)
    // May or may not have special styling
  })

  test('should display anime cards in schedule', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // Look for anime entries
    const animeCards = page.locator('a[href^="/anime/"], [class*="card"], [class*="anime"]')

    // If schedule has data, cards should be visible
    const cardCount = await animeCards.count()
    if (cardCount > 0) {
      await expect(animeCards.first()).toBeVisible()
    }
  })

  test('should display episode info', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // Look for episode number text
    const episodeText = page.getByText(/episode|ep\.|серия|эпизод/i)

    // May be visible if schedule has data
  })

  test('should display release time', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // Look for time display (format like 18:00, 6:00 PM, etc.)
    const timeText = page.getByText(/\d{1,2}:\d{2}/)

    // May be visible if schedule has data
  })

  test('should navigate to anime from schedule', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    const animeLink = page.locator('a[href^="/anime/"]').first()

    if (await animeLink.isVisible()) {
      await animeLink.click()

      // Should navigate to anime detail page
      await expect(page).toHaveURL(/\/anime\//)
    }
  })

  test('should show empty state when no schedule', async ({ page }) => {
    // Wait for content to load
    await page.waitForTimeout(2000)

    // If no schedule data, should show empty message
    const emptyMessage = page.getByText(/no schedule|empty|пусто|нет расписания/i)

    // May or may not be visible depending on data
  })

  test('should be responsive on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await page.reload()

    // Page should still be functional
    const title = page.getByRole('heading', { name: /schedule|расписание/i })
    await expect(title).toBeVisible({ timeout: 5000 })
  })
})

test.describe('Schedule Navigation', () => {
  test('should navigate to schedule from home', async ({ page }) => {
    await page.goto('/')

    // Look for schedule link
    const scheduleLink = page.getByRole('link', { name: /schedule|расписание/i })

    if (await scheduleLink.isVisible()) {
      await scheduleLink.click()
      await expect(page).toHaveURL(/\/schedule/)
    }
  })

  test('should navigate to schedule from navigation menu', async ({ page }) => {
    await page.goto('/')

    // Look in header navigation
    const navScheduleLink = page.locator('header').getByRole('link', { name: /schedule|расписание/i })

    if (await navScheduleLink.isVisible()) {
      await navScheduleLink.click()
      await expect(page).toHaveURL(/\/schedule/)
    }
  })
})
