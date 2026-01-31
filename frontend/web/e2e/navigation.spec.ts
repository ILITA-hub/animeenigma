import { test, expect } from '@playwright/test'

test.describe('Navigation', () => {
  test('should navigate between pages', async ({ page }) => {
    await page.goto('/')

    // Navigate to Browse
    await page.getByRole('link', { name: /каталог|catalog/i }).first().click()
    await expect(page).toHaveURL(/\/browse/)

    // Navigate back to Home
    await page.getByRole('link', { name: /главная|home/i }).first().click()
    await expect(page).toHaveURL('/')
  })

  test('should display mobile navigation on small screens', async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 })
    await page.goto('/')

    // Mobile nav should be visible
    const mobileNav = page.locator('nav.fixed.bottom-0')
    await expect(mobileNav).toBeVisible()
  })

  test('should hide desktop nav on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await page.goto('/')

    // Desktop nav links should be hidden
    const desktopNavLinks = page.locator('header .hidden.md\\:flex').first()
    await expect(desktopNavLinks).toBeHidden()
  })
})
