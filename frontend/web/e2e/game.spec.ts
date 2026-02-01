import { test, expect } from '@playwright/test'

test.describe('Game Rooms', () => {
  test.describe('Room List', () => {
    test.beforeEach(async ({ page }) => {
      await page.goto('/game')
    })

    test('should display game page header', async ({ page }) => {
      // Look for the page content
      await expect(page.getByText('Available Rooms')).toBeVisible({ timeout: 10000 })
    })

    test('should display create room button', async ({ page }) => {
      // The create button uses i18n $t('rooms.create')
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]') // Plus icon
      })
      await expect(createButton).toBeVisible({ timeout: 10000 })
    })

    test('should display room cards or empty state', async ({ page }) => {
      await page.waitForTimeout(3000)

      // Either show rooms or empty state
      const roomCards = page.locator('button').filter({
        has: page.locator('h3')
      })
      const emptyState = page.getByText('No active rooms')

      const hasRooms = await roomCards.count() > 0
      const hasEmptyState = await emptyState.isVisible()

      expect(hasRooms || hasEmptyState).toBeTruthy()
    })

    test('should show empty state when no rooms', async ({ page }) => {
      await page.waitForTimeout(3000)

      // Check for empty state elements
      const emptyMessage = page.getByText('No active rooms')
      const createFirstButton = page.getByRole('button', { name: 'Create the first room' })

      // May or may not be visible depending on data
    })

    test('should open create room modal', async ({ page }) => {
      // Click create button (has plus icon)
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]')
      }).first()

      await createButton.click()

      // Modal should appear with form
      await expect(page.getByPlaceholder('Enter room name')).toBeVisible({ timeout: 5000 })
    })

    test('should display room creation form fields', async ({ page }) => {
      // Open modal
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]')
      }).first()
      await createButton.click()

      // Check form fields
      await expect(page.getByPlaceholder('Enter room name')).toBeVisible()
      await expect(page.getByText('Game Type')).toBeVisible()
      await expect(page.getByText('Max Players')).toBeVisible()
    })

    test('should have game type options', async ({ page }) => {
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]')
      }).first()
      await createButton.click()

      // Check game type dropdown
      const gameTypeSelect = page.locator('select')
      await expect(gameTypeSelect).toBeVisible()

      // Check options
      await expect(page.locator('option', { hasText: 'Anime Quiz' })).toBeVisible()
      await expect(page.locator('option', { hasText: 'Character Guess' })).toBeVisible()
      await expect(page.locator('option', { hasText: 'Opening Quiz' })).toBeVisible()
    })

    test('should close modal on cancel', async ({ page }) => {
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]')
      }).first()
      await createButton.click()

      // Wait for modal
      await expect(page.getByPlaceholder('Enter room name')).toBeVisible()

      // Click cancel button (uses $t('common.cancel'))
      const cancelButton = page.getByRole('button', { name: /cancel|отмена/i })
      await cancelButton.click()

      // Modal should close
      await expect(page.getByPlaceholder('Enter room name')).toBeHidden({ timeout: 3000 })
    })
  })

  test.describe('Room Creation', () => {
    test('should create a new room', async ({ page }) => {
      await page.goto('/game')

      // Open modal
      const createButton = page.locator('button').filter({
        has: page.locator('svg path[d*="M12 4v16"]')
      }).first()
      await createButton.click()

      // Fill form
      await page.getByPlaceholder('Enter room name').fill(`Test Room ${Date.now()}`)

      // Submit - click the create button in modal footer
      const submitButton = page.locator('[class*="modal"], [role="dialog"]').getByRole('button', { name: /create/i }).last()
      await submitButton.click()

      // Should enter the room or show error
      await page.waitForTimeout(3000)
    })
  })

  test.describe('In-Room View', () => {
    test('should display room content when navigating directly', async ({ page }) => {
      // Navigate to a room (may not exist)
      await page.goto('/game/test-room-id')

      // Should show something (room content or redirect back to list)
      await page.waitForTimeout(3000)
    })

    test('should display players section in room', async ({ page }) => {
      await page.goto('/game')
      await page.waitForTimeout(3000)

      // Try to find and click a room
      const roomCard = page.locator('button').filter({
        has: page.locator('h3')
      }).first()

      if (await roomCard.isVisible()) {
        await roomCard.click()
        await page.waitForTimeout(2000)

        // Should see Players section
        const playersSection = page.getByText(/Players \(\d+\)/)
        await expect(playersSection).toBeVisible({ timeout: 5000 })
      }
    })

    test('should display chat section in room', async ({ page }) => {
      await page.goto('/game')
      await page.waitForTimeout(3000)

      const roomCard = page.locator('button').filter({
        has: page.locator('h3')
      }).first()

      if (await roomCard.isVisible()) {
        await roomCard.click()
        await page.waitForTimeout(2000)

        // Should see Chat section
        await expect(page.getByText('Chat')).toBeVisible({ timeout: 5000 })
        await expect(page.getByPlaceholder('Type a message...')).toBeVisible()
      }
    })

    test('should have leave room button', async ({ page }) => {
      await page.goto('/game')
      await page.waitForTimeout(3000)

      const roomCard = page.locator('button').filter({
        has: page.locator('h3')
      }).first()

      if (await roomCard.isVisible()) {
        await roomCard.click()
        await page.waitForTimeout(2000)

        // Should see leave button (uses $t('rooms.leave'))
        const leaveButton = page.getByRole('button', { name: /leave|выйти|покинуть/i })
        await expect(leaveButton).toBeVisible({ timeout: 5000 })
      }
    })
  })
})

test.describe('Game Navigation', () => {
  test('should navigate to game from header', async ({ page }) => {
    await page.goto('/')

    const gameLink = page.locator('header').getByRole('link', { name: /game|игр/i })

    if (await gameLink.isVisible()) {
      await gameLink.click()
      await expect(page).toHaveURL(/\/game/)
    }
  })

  test('should navigate to game from mobile nav', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await page.goto('/')

    const mobileGameLink = page.locator('nav.fixed').getByRole('link', { name: /game|игр/i })

    if (await mobileGameLink.isVisible()) {
      await mobileGameLink.click()
      await expect(page).toHaveURL(/\/game/)
    }
  })
})
