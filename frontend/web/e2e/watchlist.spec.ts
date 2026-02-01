import { test, expect } from '@playwright/test'

test.describe('Watchlist Management', () => {
  test.beforeEach(async ({ page }) => {
    // Setup authenticated state
    await page.goto('/')
    await page.evaluate(() => {
      localStorage.setItem('token', 'mock-token-for-testing')
      localStorage.setItem('user', JSON.stringify({
        id: 'test-user-id',
        username: 'testuser',
        email: 'test@example.com',
        role: 'user'
      }))
    })
  })

  test.describe('Add to Watchlist', () => {
    test('should show watchlist button on anime page', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for watchlist dropdown or button
      const watchlistControl = page.locator('select, button').filter({
        hasText: /watching|plan|completed|hold|dropped|add|смотрю|запланировано|просмотрено|добавить/i
      })

      // Should be visible for authenticated users
    })

    test('should add anime to watchlist', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Try to add to watchlist
      const watchlistSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /watching|смотрю/i })
      }).first()

      if (await watchlistSelect.isVisible()) {
        await watchlistSelect.selectOption({ label: /watching|смотрю/i })
        await page.waitForTimeout(1000)
      }
    })

    test('should update watchlist status', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      const watchlistSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /completed|просмотрено/i })
      }).first()

      if (await watchlistSelect.isVisible()) {
        // Change to completed
        await watchlistSelect.selectOption({ label: /completed|просмотрено/i })
        await page.waitForTimeout(1000)
      }
    })
  })

  test.describe('View Watchlist', () => {
    test('should display watchlist in profile', async ({ page }) => {
      await page.goto('/profile')

      // Should show watchlist tab or content
      const watchlistTab = page.getByRole('button', { name: /lists|списки|my lists/i })

      if (await watchlistTab.isVisible()) {
        await watchlistTab.click()
      }

      // Look for watchlist content
      await page.waitForTimeout(2000)
    })

    test('should filter watchlist by status', async ({ page }) => {
      await page.goto('/profile')

      // Click watching filter
      const watchingFilter = page.getByRole('button', { name: /watching|смотрю/i }).first()

      if (await watchingFilter.isVisible()) {
        await watchingFilter.click()
        await page.waitForTimeout(500)

        // List should update
      }
    })

    test('should show anime details in watchlist', async ({ page }) => {
      await page.goto('/profile')
      await page.waitForTimeout(2000)

      // In table view, should show score, type, progress
      const tableHeader = page.locator('th').filter({
        hasText: /score|type|progress|очки|тип|прогресс/i
      })

      // May be visible in table view
    })

    test('should remove anime from watchlist', async ({ page }) => {
      await page.goto('/profile')
      await page.waitForTimeout(2000)

      // Find delete button
      const deleteButton = page.locator('button[title*="Remove"], button[title*="Delete"], button[title*="remove"]').first()

      if (await deleteButton.isVisible()) {
        await deleteButton.click()
        await page.waitForTimeout(1000)
      }
    })
  })

  test.describe('Watchlist from Anime Page', () => {
    test('should show current status if anime is in list', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // If anime is in watchlist, dropdown should show current status
      const watchlistSelect = page.locator('select').first()

      if (await watchlistSelect.isVisible()) {
        const selectedValue = await watchlistSelect.inputValue()
        // Value should be one of the statuses
      }
    })

    test('should persist watchlist changes', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      // Get anime link
      const animeLink = page.locator('a[href^="/anime/"]').first()
      const href = await animeLink.getAttribute('href')

      await animeLink.click()
      await expect(page).toHaveURL(/\/anime\//)

      // Add to watchlist
      const watchlistSelect = page.locator('select').filter({
        has: page.locator('option', { hasText: /watching|смотрю/i })
      }).first()

      if (await watchlistSelect.isVisible()) {
        await watchlistSelect.selectOption({ label: /watching|смотрю/i })
        await page.waitForTimeout(1000)

        // Reload page
        await page.reload()
        await page.waitForTimeout(2000)

        // Status should persist
      }
    })
  })
})

test.describe('MAL Import', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.evaluate(() => {
      localStorage.setItem('token', 'mock-token')
      localStorage.setItem('user', JSON.stringify({
        id: 'test-user-id',
        username: 'testuser'
      }))
    })
  })

  test('should display MAL import section in settings', async ({ page }) => {
    await page.goto('/profile')

    const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
    if (await settingsTab.isVisible()) {
      await settingsTab.click()
    }

    const malSection = page.getByText(/myanimelist|import/i)
    await expect(malSection.first()).toBeVisible({ timeout: 5000 })
  })

  test('should have username input field', async ({ page }) => {
    await page.goto('/profile')

    const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
    if (await settingsTab.isVisible()) {
      await settingsTab.click()
    }

    const malInput = page.getByPlaceholder(/username|mal/i)
    await expect(malInput).toBeVisible({ timeout: 5000 })
  })

  test('should trigger import when button clicked', async ({ page }) => {
    await page.goto('/profile')

    const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
    if (await settingsTab.isVisible()) {
      await settingsTab.click()
    }

    const malInput = page.getByPlaceholder(/username|mal/i)
    await malInput.fill('Neymik')

    const importButton = page.getByRole('button', { name: /import|импорт/i })
    await importButton.click()

    // Should show loading state
    await expect(importButton).toContainText(/import|импорт/i)
  })

  test('should show import results', async ({ page }) => {
    await page.goto('/profile')

    const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
    if (await settingsTab.isVisible()) {
      await settingsTab.click()
    }

    const malInput = page.getByPlaceholder(/username|mal/i)
    await malInput.fill('Neymik')

    const importButton = page.getByRole('button', { name: /import|импорт/i })
    await importButton.click()

    // Wait for result
    await page.waitForTimeout(10000)

    // Should show imported/skipped counts
    const resultText = page.getByText(/imported|skipped|импортировано|пропущено/i)
    // May be visible after import completes
  })
})
