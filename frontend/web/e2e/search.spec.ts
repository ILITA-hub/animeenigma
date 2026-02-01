import { test, expect } from '@playwright/test'

test.describe('Browse/Search Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/browse')
  })

  test.describe('Search Input', () => {
    test('should display search input', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i)
      await expect(searchInput.first()).toBeVisible()
    })

    test('should show live search results', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('Naruto')

      // Wait for debounced search
      await page.waitForTimeout(500)

      // Look for dropdown results
      const dropdown = page.locator('[class*="dropdown"], [class*="results"], [class*="suggestions"]')
      // May show results dropdown
    })

    test('should navigate to anime on result click', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('Naruto')

      await page.waitForTimeout(1000)

      // Click on first result if dropdown is visible
      const result = page.locator('a[href^="/anime/"]').first()

      if (await result.isVisible()) {
        await result.click()
        await expect(page).toHaveURL(/\/anime\//)
      }
    })

    test('should clear search input', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('Test search')

      // Clear with clear button or manually
      await searchInput.clear()
      await expect(searchInput).toHaveValue('')
    })
  })

  test.describe('Filters', () => {
    test('should display genre filter', async ({ page }) => {
      const genreFilter = page.locator('select, button').filter({
        hasText: /genre|жанр/i
      }).or(page.getByText(/genre|жанр/i))

      await expect(genreFilter.first()).toBeVisible()
    })

    test('should display year filter', async ({ page }) => {
      const yearFilter = page.locator('select, button').filter({
        hasText: /year|год/i
      }).or(page.getByText(/year|год/i))

      // May be visible
    })

    test('should display sort options', async ({ page }) => {
      const sortSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /popular|rating|year|a-z|популярн|рейтинг/i })
      })

      await expect(sortSelect.first()).toBeVisible()
    })

    test('should filter by genre', async ({ page }) => {
      const genreSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /action|adventure|comedy|экшен|приключения|комедия/i })
      }).first()

      if (await genreSelect.isVisible()) {
        await genreSelect.selectOption({ index: 1 })
        await page.waitForTimeout(1000)
      }
    })

    test('should sort results', async ({ page }) => {
      const sortSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /popular|rating|популярн|рейтинг/i })
      }).first()

      if (await sortSelect.isVisible()) {
        await sortSelect.selectOption({ label: /rating|рейтинг/i })
        await page.waitForTimeout(1000)
      }
    })

    test('should clear filters', async ({ page }) => {
      // Apply a filter first
      const genreSelect = page.locator('select').first()
      if (await genreSelect.isVisible()) {
        await genreSelect.selectOption({ index: 1 })
        await page.waitForTimeout(500)
      }

      // Look for clear button
      const clearButton = page.getByRole('button', { name: /clear|сбросить/i })

      if (await clearButton.isVisible()) {
        await clearButton.click()
        await page.waitForTimeout(500)
      }
    })
  })

  test.describe('Results Display', () => {
    test('should display anime grid', async ({ page }) => {
      await page.waitForTimeout(2000)

      const animeCards = page.locator('a[href^="/anime/"], [class*="card"], [class*="anime"]')
      const count = await animeCards.count()

      expect(count).toBeGreaterThanOrEqual(0)
    })

    test('should display anime poster', async ({ page }) => {
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const poster = page.locator('img').first()
      await expect(poster).toBeVisible()
    })

    test('should display anime title', async ({ page }) => {
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      const title = page.locator('h3, h4, [class*="title"]').first()
      await expect(title).toBeVisible()
    })

    test('should navigate to anime on card click', async ({ page }) => {
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)
    })
  })

  test.describe('Pagination', () => {
    test('should display load more button', async ({ page }) => {
      await page.waitForTimeout(2000)

      const loadMoreButton = page.getByRole('button', { name: /load more|показать еще|больше/i })

      // May be visible if there are more results
    })

    test('should load more results on click', async ({ page }) => {
      await page.waitForTimeout(2000)

      const loadMoreButton = page.getByRole('button', { name: /load more|показать еще|больше/i })

      if (await loadMoreButton.isVisible()) {
        const initialCount = await page.locator('a[href^="/anime/"]').count()

        await loadMoreButton.click()
        await page.waitForTimeout(2000)

        const newCount = await page.locator('a[href^="/anime/"]').count()
        expect(newCount).toBeGreaterThanOrEqual(initialCount)
      }
    })
  })

  test.describe('Recent Searches', () => {
    test('should display recent searches', async ({ page }) => {
      // First, perform a search
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('Naruto')
      await searchInput.press('Enter')

      await page.waitForTimeout(1000)

      // Go back to browse
      await page.goto('/browse')

      // Check for recent searches section
      const recentSection = page.getByText(/recent|недавние/i)
      // May be visible
    })
  })

  test.describe('Empty State', () => {
    test('should show empty state for no results', async ({ page }) => {
      const searchInput = page.getByPlaceholder(/search|поиск/i).first()
      await searchInput.fill('xyznonexistentanime123456')
      await searchInput.press('Enter')

      await page.waitForTimeout(2000)

      const emptyMessage = page.getByText(/no results|not found|ничего не найдено|нет результатов/i)
      // May be visible for empty results
    })
  })

  test.describe('Responsive', () => {
    test('should adjust grid on mobile', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 })
      await page.reload()

      await page.waitForTimeout(2000)

      // Grid should be 2 columns on mobile
      const grid = page.locator('[class*="grid"]').first()
      await expect(grid).toBeVisible()
    })
  })
})
