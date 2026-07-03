import { test, expect, type APIRequestContext } from '@playwright/test'

// PWA smoke: requires a PRODUCTION build being served (SW registration is
// PROD-gated). Skips itself on dev servers where /sw.js isn't a real script.
//
// The naive "just check .ok()" probe isn't enough on this project's dev
// server: vite's SPA history fallback answers EVERY unmatched path
// (including /sw.js) with a 200 text/html index.html, so a bare ok() check
// never trips. Prod nginx serves the real sw.js as application/javascript —
// require both a 2xx status AND a non-HTML content-type before proceeding.
async function isRealServiceWorker(request: APIRequestContext): Promise<boolean> {
  const resp = await request.get('/sw.js')
  if (!resp.ok()) return false
  const contentType = resp.headers()['content-type'] ?? ''
  return !contentType.includes('text/html')
}

test.describe('PWA shell', () => {
  test('manifest is served and linked', async ({ page }) => {
    // The default e2e webServer is the DEV server, where the PWA plugin emits
    // neither sw.js nor the manifest link — probe and self-skip, same as below.
    test.skip(!(await isRealServiceWorker(page.request)), 'no sw.js — dev server / SW not built')
    await page.goto('/')
    const href = await page.locator('link[rel="manifest"]').getAttribute('href')
    expect(href).toBeTruthy()
    const resp = await page.request.get(href!)
    expect(resp.ok()).toBeTruthy()
    const manifest = await resp.json()
    expect(manifest.name).toBe('AnimeEnigma')
    expect(manifest.display).toBe('standalone')
  })

  test('service worker takes control and app shell survives offline reload', async ({ page, context }) => {
    test.skip(!(await isRealServiceWorker(page.request)), 'no sw.js — dev server / SW not built')
    await page.goto('/')
    await page.waitForFunction(() => !!navigator.serviceWorker?.controller, undefined, { timeout: 20_000 })
    await context.setOffline(true)
    await page.reload()
    // App.vue's template root also carries id="app" (nested inside the
    // index.html mount div of the same id) — .first() keeps the locator
    // strict-mode-safe against that pre-existing duplicate-id structure.
    await expect(page.locator('#app').first()).not.toBeEmpty()
    // downloads page is part of the shell — reachable offline with empty state
    await page.goto('/downloads')
    await expect(page.locator('#app').first()).not.toBeEmpty()
    await context.setOffline(false)
  })
})
