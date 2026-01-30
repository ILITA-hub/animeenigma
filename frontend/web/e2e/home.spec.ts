import { test, expect } from '@playwright/test'

test.describe('Home Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test('should display the hero section', async ({ page }) => {
    // Check hero is visible
    await expect(page.locator('section').first()).toBeVisible()

    // Check CTA buttons exist
    await expect(page.getByRole('link', { name: /каталог|catalog|browse/i })).toBeVisible()
  })

  test('should display navigation', async ({ page }) => {
    // Desktop nav should be visible on large screens
    await expect(page.locator('header')).toBeVisible()
  })

  test('should navigate to browse page', async ({ page }) => {
    await page.getByRole('link', { name: /каталог|catalog|browse/i }).first().click()
    await expect(page).toHaveURL(/\/browse/)
  })

  test('should display trending section', async ({ page }) => {
    // Look for trending section heading
    const trendingSection = page.getByText(/trending|тренде/i)
    await expect(trendingSection.first()).toBeVisible()
  })
})
