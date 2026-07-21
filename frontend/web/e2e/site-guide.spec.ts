import { expect, test } from '@playwright/test'

test.describe('secret interactive site guide', () => {
  test('launches from hidden tips and completes the real interface tour', async ({ page }) => {
    await page.goto('/tips')
    await page.getByTestId('site-guide-launch').click()

    await expect(page).toHaveURL(/^.*\/$/)
    await expect(page.getByTestId('site-guide-panel')).toBeVisible()
    await expect(page.getByTestId('site-guide-spotlight')).toBeVisible()

    for (let step = 1; step < 6; step += 1) {
      await page.getByTestId('site-guide-next').click()
      await expect(page.getByTestId('site-guide-panel')).toBeVisible()
    }

    await page.getByTestId('site-guide-next').click()
    await expect(page.getByTestId('site-guide-panel')).toBeHidden()
  })
})
