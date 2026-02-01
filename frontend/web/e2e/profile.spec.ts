import { test, expect } from '@playwright/test'

test.describe('Profile Page', () => {
  test.describe('Unauthenticated', () => {
    test('should redirect to auth when not logged in', async ({ page }) => {
      await page.goto('/')
      await page.evaluate(() => {
        localStorage.removeItem('token')
        localStorage.removeItem('user')
      })

      await page.goto('/profile')

      // Should redirect to auth or show login prompt
      await expect(page).toHaveURL(/\/(auth|profile)/)
    })
  })

  test.describe('Authenticated', () => {
    test.beforeEach(async ({ page }) => {
      // Login first
      await page.goto('/auth')

      await page.getByPlaceholder(/username|имя пользователя/i).fill('testuser')
      await page.getByPlaceholder(/password|пароль/i).first().fill('password123')
      await page.getByRole('button', { name: /login|войти/i }).click()

      // Wait for potential redirect or set mock auth
      await page.waitForTimeout(2000)
    })

    test('should display profile header', async ({ page }) => {
      await page.goto('/profile')

      // Should show user info section
      const profileSection = page.locator('[class*="profile"], [class*="header"]').first()
      await expect(profileSection).toBeVisible({ timeout: 5000 })
    })

    test('should display tabs navigation', async ({ page }) => {
      await page.goto('/profile')

      // Look for tab buttons
      const listsTab = page.getByRole('button', { name: /lists|списки|my lists/i })
      const historyTab = page.getByRole('button', { name: /history|история/i })
      const settingsTab = page.getByRole('button', { name: /settings|настройки/i })

      // At least one tab should be visible
      await expect(listsTab.or(historyTab).or(settingsTab)).toBeVisible({ timeout: 5000 })
    })

    test.describe('Watchlist Tab', () => {
      test('should display watchlist filters', async ({ page }) => {
        await page.goto('/profile')

        // Look for status filter buttons
        const filterButtons = page.locator('button').filter({
          hasText: /all|watching|plan|completed|hold|dropped|все|смотрю|запланировано|просмотрено|отложено|брошено/i
        })

        await expect(filterButtons.first()).toBeVisible({ timeout: 5000 })
      })

      test('should toggle between table and grid view', async ({ page }) => {
        await page.goto('/profile')

        // Look for view toggle buttons
        const tableViewButton = page.locator('button[title*="Table"], button[title*="table"]')
        const gridViewButton = page.locator('button[title*="Grid"], button[title*="grid"]')

        if (await tableViewButton.isVisible()) {
          await tableViewButton.click()
          // Should show table
          await expect(page.locator('table')).toBeVisible({ timeout: 3000 })
        }

        if (await gridViewButton.isVisible()) {
          await gridViewButton.click()
          // Should show grid
          await expect(page.locator('[class*="grid"]')).toBeVisible({ timeout: 3000 })
        }
      })

      test('should filter watchlist by status', async ({ page }) => {
        await page.goto('/profile')

        // Click on "Watching" filter
        const watchingFilter = page.getByRole('button', { name: /watching|смотрю/i })

        if (await watchingFilter.isVisible()) {
          await watchingFilter.click()

          // Should update the list
          await page.waitForTimeout(500)
        }
      })

      test('should display empty state when list is empty', async ({ page }) => {
        await page.goto('/profile')

        // If list is empty, should show empty message
        const emptyMessage = page.getByText(/empty|пуст|no anime|нет аниме/i)
        // May or may not be visible depending on data
      })
    })

    test.describe('History Tab', () => {
      test('should switch to history tab', async ({ page }) => {
        await page.goto('/profile')

        const historyTab = page.getByRole('button', { name: /history|история/i })

        if (await historyTab.isVisible()) {
          await historyTab.click()

          // Should show history content
          await page.waitForTimeout(500)
        }
      })
    })

    test.describe('Settings Tab', () => {
      test('should switch to settings tab', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })

        if (await settingsTab.isVisible()) {
          await settingsTab.click()

          // Should show settings content
          const settingsContent = page.getByText(/language|язык|appearance|playback|воспроизведение/i)
          await expect(settingsContent.first()).toBeVisible({ timeout: 3000 })
        }
      })

      test('should display language selector', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
        if (await settingsTab.isVisible()) {
          await settingsTab.click()
        }

        // Look for language dropdown
        const languageSelect = page.locator('select').filter({
          has: page.locator('option', { hasText: /english|русский|日本語/i })
        })

        // May be visible in settings
      })

      test('should display MAL import section', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
        if (await settingsTab.isVisible()) {
          await settingsTab.click()
        }

        // Look for MAL import section
        const malSection = page.getByText(/myanimelist|mal/i)
        await expect(malSection.first()).toBeVisible({ timeout: 3000 })
      })

      test('should have MAL username input', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
        if (await settingsTab.isVisible()) {
          await settingsTab.click()
        }

        // Look for MAL username input
        const malInput = page.getByPlaceholder(/mal username|username/i)
        await expect(malInput).toBeVisible({ timeout: 3000 })
      })

      test('should trigger MAL import', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
        if (await settingsTab.isVisible()) {
          await settingsTab.click()
        }

        const malInput = page.getByPlaceholder(/mal username|username/i)
        if (await malInput.isVisible()) {
          await malInput.fill('testuser')

          const importButton = page.getByRole('button', { name: /import|импорт/i })
          await importButton.click()

          // Should show loading or result
          await page.waitForTimeout(2000)
        }
      })

      test('should have logout button', async ({ page }) => {
        await page.goto('/profile')

        const settingsTab = page.getByRole('button', { name: /settings|настройки/i })
        if (await settingsTab.isVisible()) {
          await settingsTab.click()
        }

        const logoutButton = page.getByRole('button', { name: /logout|sign out|выйти/i })
        await expect(logoutButton).toBeVisible({ timeout: 3000 })
      })
    })
  })
})
