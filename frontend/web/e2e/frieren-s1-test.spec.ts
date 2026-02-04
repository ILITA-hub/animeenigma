import { test } from '@playwright/test'

test('test Frieren S1 episode 4', async ({ page }) => {
  await page.goto('/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623')
  await page.waitForTimeout(2000)
  
  // Click HiAnime tab
  const hiAnimeTab = page.locator('button').filter({ hasText: /hianime/i })
  if (await hiAnimeTab.isVisible()) {
    await hiAnimeTab.click()
    await page.waitForTimeout(3000)
  }
  
  // Click episode 4
  const ep4 = page.locator('.hianime-player button').filter({ hasText: /^4$/ }).first()
  if (await ep4.isVisible()) {
    console.log('Clicking episode 4...')
    await ep4.click()
    await page.waitForTimeout(5000)
  } else {
    console.log('Episode 4 button not found')
    // List all buttons
    const btns = page.locator('.hianime-player button')
    const count = await btns.count()
    console.log(`Total buttons: ${count}`)
    for (let i = 0; i < Math.min(count, 15); i++) {
      const txt = await btns.nth(i).textContent()
      console.log(`  Button ${i}: "${txt}"`)
    }
  }
  
  // Check for errors
  const error = page.locator('.hianime-player').locator('text=Ошибка')
  const hasError = await error.isVisible()
  console.log(`Error visible: ${hasError}`)
  
  // Check for iframe
  const iframe = page.locator('.hianime-player iframe')
  const hasIframe = await iframe.isVisible()
  console.log(`Iframe visible: ${hasIframe}`)
  
  if (hasIframe) {
    const src = await iframe.getAttribute('src')
    console.log(`Iframe src: ${src}`)
  }
  
  // Check for video
  const video = page.locator('.hianime-player video')
  const hasVideo = await video.isVisible()
  console.log(`Video visible: ${hasVideo}`)
  
  await page.screenshot({ path: 'test-results/frieren-s1-ep4.png', fullPage: true })
})
