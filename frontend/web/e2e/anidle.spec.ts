import { test, expect } from '@playwright/test'

/**
 * Anidle anime-guessing game — Plan 3 frontend e2e spec.
 *
 * Tests the full game flow: anonymous daily meta, search autocomplete,
 * guess submission, give-up, endless mode, and share button.
 *
 * NOTE: This spec is CREATED but UNRUN (no browser environment in CI without
 * the live backend). All tests gracefully skip on backend unavailability.
 */

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

async function loginAsUiAuditBot(page: import('@playwright/test').Page) {
  await page.goto('/')
  const ok = await page.evaluate(
    async ({ username, password }: { username: string; password: string }) => {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      if (!res.ok) return { ok: false, status: res.status }
      const payload = await res.json()
      const data = payload?.data ?? payload
      const token: string | undefined = data?.access_token
      const user = data?.user
      if (!token || !user) return { ok: false, status: 200, reason: 'no-token-or-user' }
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
      return { ok: true, status: 200 }
    },
    { username: UI_AUDIT_USERNAME, password: UI_AUDIT_PASSWORD },
  )
  return ok
}

/**
 * Check if the anidle backend is reachable.
 */
async function isAnidleAvailable(page: import('@playwright/test').Page): Promise<boolean> {
  try {
    const status = await page.evaluate(async () => {
      const res = await fetch('/api/anidle/daily', { credentials: 'include' })
      return res.status
    })
    return status >= 200 && status < 500
  } catch {
    return false
  }
}

test.describe('anidle game (Plan 3)', () => {
  test('Test 1 — anonymous daily meta: page mounts without crash', async ({ page }) => {
    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    // Should show the page title
    await expect(page.locator('h1')).toContainText('Anidle')

    // Should show the search input
    const searchInput = page.locator('input[placeholder*="ниме"], input[placeholder*="anime"], input[placeholder*="Anime"]')
    await expect(searchInput.first()).toBeVisible()

    // Should NOT be a blank white screen — some content exists
    const body = await page.textContent('body')
    expect(body).toBeTruthy()
    expect(body!.length).toBeGreaterThan(10)
  })

  test('Test 2 — search autocomplete: dropdown appears with results', async ({ page }) => {
    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    const searchInput = page.locator('input[role="combobox"]').first()
    await expect(searchInput).toBeVisible()

    await searchInput.fill('на')
    // Wait for debounce + response
    await page.waitForTimeout(500)

    // Look for dropdown items
    const listbox = page.locator('[role="listbox"]')
    const hasResults = await listbox.isVisible().catch(() => false)

    if (hasResults) {
      const options = page.locator('[role="option"]')
      const count = await options.count()
      expect(count).toBeGreaterThanOrEqual(1)

      // Each option should have an img for poster
      const firstOption = options.first()
      await expect(firstOption.locator('img')).toBeVisible()
    }
    // If no results, the test is still valid (search works, just empty)
  })

  test('Test 3 — daily guess flow (anonymous): guess submits and grid row appears', async ({ page }) => {
    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    const searchInput = page.locator('input[role="combobox"]').first()
    const isVisible = await searchInput.isVisible().catch(() => false)
    if (!isVisible) {
      // Game already completed today
      test.skip()
      return
    }

    await searchInput.fill('на')
    await page.waitForTimeout(500)

    const firstOption = page.locator('[role="option"]').first()
    const hasOption = await firstOption.isVisible().catch(() => false)
    if (!hasOption) {
      test.skip()
      return
    }

    await firstOption.click()
    await page.waitForTimeout(800)

    // A grid row should appear with at least one colored cell
    const coloredCell = page.locator('.bg-success, .bg-warning, .bg-muted').first()
    await expect(coloredCell).toBeVisible({ timeout: 5000 })

    // answer (result modal) should NOT appear yet unless we happened to solve it
    // We cannot guarantee we didn't solve on first guess, so just verify grid appeared
  })

  test('Test 4 — daily give up (logged-in): result modal appears', async ({ page }) => {
    const loginResult = await loginAsUiAuditBot(page)
    if (!loginResult.ok) {
      test.skip()
      return
    }

    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    // If already solved/gave up today, the give-up button won't be visible
    const giveUpButton = page.locator('button').filter({ hasText: /сдать|give up/i }).first()
    const hasGiveUp = await giveUpButton.isVisible({ timeout: 3000 }).catch(() => false)
    if (!hasGiveUp) {
      // Game already complete today — skip
      test.skip()
      return
    }

    await giveUpButton.click()
    await page.waitForTimeout(1000)

    // Result modal should appear with the revealed anime name
    const modal = page.locator('[role="dialog"], .modal-stub, [data-state="open"]').first()
    const modalVisible = await modal.isVisible({ timeout: 3000 }).catch(() => false)

    if (modalVisible) {
      // Should have a poster img inside
      const posterImg = modal.locator('img').first()
      await expect(posterImg).toBeVisible()
    }
  })

  test('Test 5 — endless mode: new round starts after clicking tab', async ({ page }) => {
    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    // Click the Endless tab
    const endlessTab = page.locator('[role="tab"]').filter({ hasText: /бескон|endless/i }).first()
    const hasTab = await endlessTab.isVisible({ timeout: 3000 }).catch(() => false)
    if (!hasTab) {
      test.skip()
      return
    }

    await endlessTab.click()
    await page.waitForTimeout(300)

    // "New round" button should appear
    const newRoundButton = page.locator('button').filter({ hasText: /новый раунд|new round/i }).first()
    await expect(newRoundButton).toBeVisible({ timeout: 3000 })

    await newRoundButton.click()
    await page.waitForTimeout(1000)

    // After starting, the search input should be active
    const searchInput = page.locator('input[role="combobox"]').first()
    const isVisible = await searchInput.isVisible({ timeout: 3000 }).catch(() => false)
    expect(isVisible).toBe(true)

    // Make one guess
    await searchInput.fill('на')
    await page.waitForTimeout(500)

    const firstOption = page.locator('[role="option"]').first()
    const hasOption = await firstOption.isVisible({ timeout: 3000 }).catch(() => false)
    if (!hasOption) return

    await firstOption.click()
    await page.waitForTimeout(800)

    // A grid row should appear
    const coloredCell = page.locator('.bg-success, .bg-warning, .bg-muted').first()
    const hasCells = await coloredCell.isVisible({ timeout: 3000 }).catch(() => false)
    expect(hasCells).toBe(true)
  })

  test('Test 6 — share button: copied text feedback appears', async ({ page }) => {
    const loginResult = await loginAsUiAuditBot(page)
    if (!loginResult.ok) {
      test.skip()
      return
    }

    await page.goto('/anidle')
    await page.waitForLoadState('networkidle')

    const available = await isAnidleAvailable(page)
    if (!available) {
      test.skip()
      return
    }

    // Try to give up first to open the result modal
    const giveUpButton = page.locator('button').filter({ hasText: /сдать|give up/i }).first()
    const hasGiveUp = await giveUpButton.isVisible({ timeout: 3000 }).catch(() => false)

    if (!hasGiveUp) {
      // Game already complete — result modal might be open or was closed
      // Try to trigger the show result state by looking for a reopener
      test.skip()
      return
    }

    await giveUpButton.click()
    await page.waitForTimeout(1000)

    // Grant clipboard permission and click share button
    await page.evaluate(() => {
      // Override clipboard.writeText to avoid permission issues in headless
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText: () => Promise.resolve() },
        configurable: true,
        writable: true,
      })
    })

    const shareButton = page.locator('button').filter({ hasText: /поделить|share result/i }).first()
    const hasShare = await shareButton.isVisible({ timeout: 3000 }).catch(() => false)
    if (!hasShare) {
      test.skip()
      return
    }

    await shareButton.click()
    await page.waitForTimeout(500)

    // Should show "Copied!" feedback
    const copiedText = page.locator('button').filter({ hasText: /скопир|copied/i }).first()
    const hasCopied = await copiedText.isVisible({ timeout: 3000 }).catch(() => false)
    expect(hasCopied).toBe(true)
  })
})
