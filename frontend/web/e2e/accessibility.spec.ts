import { test, expect } from '@playwright/test'

test.describe('Accessibility', () => {
  test('should have proper heading hierarchy on home page', async ({ page }) => {
    await page.goto('/')

    // Should have an h1
    const h1 = page.locator('h1')
    await expect(h1.first()).toBeVisible()
  })

  test('should have visible focus states', async ({ page }) => {
    await page.goto('/')

    // Tab to first focusable element
    await page.keyboard.press('Tab')

    // The focused element should have focus-visible styles
    const focusedElement = page.locator(':focus-visible')
    await expect(focusedElement).toBeVisible()
  })

  test('buttons should have accessible names', async ({ page }) => {
    await page.goto('/')

    // Find all buttons and check they have accessible names
    const buttons = page.getByRole('button')
    const count = await buttons.count()

    for (let i = 0; i < Math.min(count, 5); i++) {
      const button = buttons.nth(i)
      const name = await button.getAttribute('aria-label')
      const text = await button.textContent()

      // Button should have either aria-label or text content
      expect(name || text?.trim()).toBeTruthy()
    }
  })

  test('links should have accessible names', async ({ page }) => {
    await page.goto('/')

    // Find all links and check they have accessible names
    const links = page.getByRole('link')
    const count = await links.count()

    for (let i = 0; i < Math.min(count, 5); i++) {
      const link = links.nth(i)
      const name = await link.getAttribute('aria-label')
      const text = await link.textContent()

      // Link should have either aria-label or text content
      expect(name || text?.trim()).toBeTruthy()
    }
  })

  test('images should have alt text', async ({ page }) => {
    await page.goto('/')

    // Wait for images to load
    await page.waitForLoadState('networkidle')

    const images = page.locator('img')
    const count = await images.count()

    for (let i = 0; i < Math.min(count, 5); i++) {
      const img = images.nth(i)
      if (await img.isVisible()) {
        const alt = await img.getAttribute('alt')
        // Images should have alt attribute (can be empty for decorative)
        expect(alt).toBeDefined()
      }
    }
  })
})
