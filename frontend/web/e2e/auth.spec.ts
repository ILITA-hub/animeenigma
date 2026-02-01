import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    // Clear any existing auth state
    await page.goto('/')
    await page.evaluate(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    })
  })

  test.describe('Login Page', () => {
    test('should display login form', async ({ page }) => {
      await page.goto('/auth')

      // Russian placeholders
      await expect(page.getByPlaceholder('username')).toBeVisible()
      await expect(page.getByPlaceholder('••••••••').first()).toBeVisible()
      await expect(page.getByRole('button', { name: 'Войти' })).toBeVisible()
    })

    test('should switch between login and register tabs', async ({ page }) => {
      await page.goto('/auth')

      // Click register tab (Регистрация)
      await page.getByRole('button', { name: 'Регистрация' }).click()

      // Should show confirm password field
      await expect(page.getByPlaceholder('••••••••').nth(1)).toBeVisible()

      // Click login tab (Вход)
      await page.getByRole('button', { name: 'Вход' }).click()

      // Only one password field should be visible
      await expect(page.getByPlaceholder('••••••••')).toHaveCount(1)
    })

    test('should show error for invalid credentials', async ({ page }) => {
      await page.goto('/auth')

      await page.getByPlaceholder('username').fill('invaliduser')
      await page.getByPlaceholder('••••••••').fill('wrongpassword')
      await page.getByRole('button', { name: 'Войти' }).click()

      // Wait for error message
      await expect(page.locator('.text-pink-400')).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Registration', () => {
    test('should display registration form', async ({ page }) => {
      await page.goto('/auth')
      await page.getByRole('button', { name: 'Регистрация' }).click()

      await expect(page.getByPlaceholder('username (3-32 символа)')).toBeVisible()
      await expect(page.getByPlaceholder('Минимум 6 символов')).toBeVisible()
    })

    test('should validate password confirmation', async ({ page }) => {
      await page.goto('/auth')
      await page.getByRole('button', { name: 'Регистрация' }).click()

      await page.getByPlaceholder('username (3-32 символа)').fill('testuser')
      await page.getByPlaceholder('Минимум 6 символов').fill('password123')
      await page.getByPlaceholder('••••••••').nth(1).fill('differentpassword')

      // Should show password mismatch error
      await expect(page.getByText('Пароли не совпадают')).toBeVisible({ timeout: 3000 })
    })

    test('should register new user successfully', async ({ page }) => {
      await page.goto('/auth')
      await page.getByRole('button', { name: 'Регистрация' }).click()

      const uniqueUsername = `testuser_${Date.now()}`
      await page.getByPlaceholder('username (3-32 символа)').fill(uniqueUsername)
      await page.getByPlaceholder('Минимум 6 символов').fill('password123')
      await page.getByPlaceholder('••••••••').nth(1).fill('password123')

      await page.getByRole('button', { name: 'Зарегистрироваться' }).click()

      // Should redirect to home after successful registration
      await expect(page).toHaveURL('/', { timeout: 10000 })
    })
  })

  test.describe('Social Login', () => {
    test('should display Shikimori login option', async ({ page }) => {
      await page.goto('/auth')

      // Check for Shikimori login button
      await expect(page.getByRole('button', { name: /shikimori/i })).toBeVisible()
    })
  })
})

test.describe('Authenticated User', () => {
  test('should redirect to home after login', async ({ page }) => {
    // First register a test user
    await page.goto('/auth')
    await page.getByRole('button', { name: 'Регистрация' }).click()

    const uniqueUsername = `testuser_${Date.now()}`
    await page.getByPlaceholder('username (3-32 символа)').fill(uniqueUsername)
    await page.getByPlaceholder('Минимум 6 символов').fill('password123')
    await page.getByPlaceholder('••••••••').nth(1).fill('password123')

    await page.getByRole('button', { name: 'Зарегистрироваться' }).click()

    // Should redirect to home
    await expect(page).toHaveURL('/', { timeout: 10000 })
  })

  test('should access profile page when authenticated', async ({ page }) => {
    // Register and stay logged in
    await page.goto('/auth')
    await page.getByRole('button', { name: 'Регистрация' }).click()

    const uniqueUsername = `testuser_${Date.now()}`
    await page.getByPlaceholder('username (3-32 символа)').fill(uniqueUsername)
    await page.getByPlaceholder('Минимум 6 символов').fill('password123')
    await page.getByPlaceholder('••••••••').nth(1).fill('password123')

    await page.getByRole('button', { name: 'Зарегистрироваться' }).click()
    await expect(page).toHaveURL('/', { timeout: 10000 })

    // Navigate to profile
    await page.goto('/profile')

    // Should be on profile page (not redirected to auth)
    await expect(page).toHaveURL('/profile')
  })

  test('should logout successfully', async ({ page }) => {
    // Register and login
    await page.goto('/auth')
    await page.getByRole('button', { name: 'Регистрация' }).click()

    const uniqueUsername = `testuser_${Date.now()}`
    await page.getByPlaceholder('username (3-32 символа)').fill(uniqueUsername)
    await page.getByPlaceholder('Минимум 6 символов').fill('password123')
    await page.getByPlaceholder('••••••••').nth(1).fill('password123')

    await page.getByRole('button', { name: 'Зарегистрироваться' }).click()
    await expect(page).toHaveURL('/', { timeout: 10000 })

    // Go to profile and logout
    await page.goto('/profile')

    // Click on Settings tab
    await page.getByRole('button', { name: /Settings|Настройки/i }).click()

    // Find and click logout button
    const logoutButton = page.getByRole('button', { name: /Sign Out|Выйти/i })
    await logoutButton.click()

    // Should redirect to home
    await expect(page).toHaveURL('/')

    // Token should be cleared
    const token = await page.evaluate(() => localStorage.getItem('token'))
    expect(token).toBeNull()
  })
})
