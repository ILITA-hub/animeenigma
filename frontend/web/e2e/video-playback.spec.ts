import { test, expect } from '@playwright/test'

test('Consumet video player loads and plays', async ({ page }) => {
  // Go to anime page
  await page.goto('/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25')
  
  // Wait for page to load
  await page.waitForLoadState('networkidle')
  
  // Click Consumet tab
  await page.click('button:has-text("Consumet")')
  
  // Wait for episodes to load
  await page.waitForSelector('.episode-item, button:has-text("1")', { timeout: 10000 })
  
  // Click first episode
  const episodeBtn = page.locator('button:has-text("1")').first()
  await episodeBtn.click()
  
  // Wait for video element
  const video = page.locator('video')
  await video.waitFor({ timeout: 15000 })
  
  // Check if video is playing
  await page.waitForTimeout(5000)
  
  // Get video state
  const videoState = await video.evaluate((v: HTMLVideoElement) => ({
    duration: v.duration,
    currentTime: v.currentTime,
    paused: v.paused,
    readyState: v.readyState,
    networkState: v.networkState,
    error: v.error?.message || null
  }))
  
  console.log('Video state:', videoState)
  
  // Wait and check again
  await page.waitForTimeout(3000)
  
  const videoState2 = await video.evaluate((v: HTMLVideoElement) => ({
    duration: v.duration,
    currentTime: v.currentTime,
    paused: v.paused,
    readyState: v.readyState,
    networkState: v.networkState,
    error: v.error?.message || null
  }))
  
  console.log('Video state after 3s:', videoState2)
  
  // Check if time is progressing
  expect(videoState2.currentTime).toBeGreaterThan(videoState.currentTime)
})
