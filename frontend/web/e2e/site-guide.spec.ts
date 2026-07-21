import { expect, test } from '@playwright/test'

test.describe('secret interactive site guide', () => {
  test('links part one directly to the automatically selected player tour', async ({ page }) => {
    await page.goto('/tips')
    await page.getByTestId('site-guide-launch').click()

    await expect(page).toHaveURL(/^.*\/$/)
    await expect(page.getByTestId('site-guide-panel')).toBeVisible()
    await expect(page.getByTestId('site-guide-spotlight')).toBeVisible()

    for (let step = 1; step < 6; step += 1) {
      await page.getByTestId('site-guide-next').click()
      await expect(page.getByTestId('site-guide-panel')).toBeVisible()
    }

    const panelBox = await page.getByTestId('site-guide-panel').boundingBox()
    const feedbackBox = await page.locator('[data-site-guide="feedback"]').boundingBox()
    expect(panelBox).not.toBeNull()
    expect(feedbackBox).not.toBeNull()
    expect(panelBox!.y + panelBox!.height <= feedbackBox!.y || panelBox!.y >= feedbackBox!.y + feedbackBox!.height).toBe(true)

    await page.getByTestId('site-guide-next').click()
    await expect(page).toHaveURL(/\/anime\/[^/?]+/)
    await expect(page.getByTestId('site-guide-panel')).toHaveAttribute('data-guide-mode', 'player')
    await expect(page.locator('[data-test="ae-player"]')).toBeVisible()
  })

  test('opens Curator Recommends or popular top-1 and demonstrates every player menu', async ({ page }) => {
    // The guide must explain AePlayer even when this browser normally prefers
    // the Classic Kodik fallback; the persisted preference itself stays intact.
    await page.addInitScript(() => localStorage.setItem('classic_kodik_selected', 'true'))
    await page.goto('/tips')
    await page.getByTestId('player-guide-launch').click()

    await expect(page).toHaveURL(/\/anime\/[^/?]+/)
    await expect(page.getByTestId('site-guide-panel')).toHaveAttribute('data-guide-mode', 'player')
    await expect(page.getByTestId('site-guide-spotlight')).toBeVisible()
    await expect(page.locator('[data-test="ae-player"]')).toBeVisible()

    for (const menu of ['episodes', 'source', 'subs', 'settings']) {
      await page.getByTestId('site-guide-next').click()
      await expect(page.locator(`[data-site-guide="player-menu-${menu}"]`)).toBeVisible()
    }

    await page.getByTestId('site-guide-next').click()
    await expect(page.locator('[data-site-guide^="player-menu-"]')).toHaveCount(0)
    await page.getByTestId('site-guide-next').click()
    await expect(page.getByTestId('site-guide-panel')).toBeHidden()
  })
})
