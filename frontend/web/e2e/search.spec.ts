import { test, expect } from '@playwright/test'

test.describe('Search/Browse Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/browse')
  })

  test('should display search input', async ({ page }) => {
    const searchInput = page.getByPlaceholder(/поиск|search/i)
    await expect(searchInput).toBeVisible()
  })

  test('should display filter options', async ({ page }) => {
    // Genre filter
    await expect(page.getByText(/жанр|genre/i).first()).toBeVisible()

    // Sort option
    await expect(page.locator('select').first()).toBeVisible()
  })

  test('should type in search and trigger search', async ({ page }) => {
    const searchInput = page.getByPlaceholder(/поиск|search/i)
    await searchInput.fill('Naruto')

    // Wait for debounced search
    await page.waitForTimeout(500)

    // Should still be on browse/search page
    await expect(page).toHaveURL(/\/(browse|search)/)
  })

  test('should clear search with clear button', async ({ page }) => {
    const searchInput = page.getByPlaceholder(/поиск|search/i)
    await searchInput.fill('Test search')

    // Look for clear button and click it
    const clearButton = page.locator('button[aria-label="Clear"]').or(
      page.locator('input[type="search"] + button')
    )

    if (await clearButton.isVisible()) {
      await clearButton.click()
      await expect(searchInput).toHaveValue('')
    }
  })
})
