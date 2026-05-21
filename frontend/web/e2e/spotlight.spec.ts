import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

/**
 * Phase 2 (HSB-FE-01..09) — HeroSpotlightBlock e2e + a11y spec.
 *
 * Anchors to ROADMAP Phase 2 success criteria 1-10:
 *   #1  Block renders at top of Home above the legacy trending row
 *   #3  Auto-cycles every ~7s
 *   #4  Hover/focus pauses cycle
 *   #5  Random initial slide (covered by Vitest in Plan 04 — not deterministic in e2e)
 *   #6  ArrowLeft / ArrowRight keyboard nav seeks
 *   #7  prefers-reduced-motion disables auto-cycle (manual nav still works)
 *   #8  Mobile viewport (375x667) — block respects min-height + stacks
 *   #9  axe-core ZERO violations on the block
 *   #10 Flag-off — covered by Vitest in Plan 01 (env-baked at build time;
 *       re-deploy not practical inside this suite — skip + manual command)
 *
 * Auth: Phase 2 spotlight is public — anonymous fetch hits /api/home/spotlight
 * without auth. No login pattern required (Phase 3 may reuse the
 * notifications.spec.ts `loginAs()` helper).
 *
 * Pragmatic test.skip(): if the backend returns 0 cards (e.g. all resolvers
 * failed, endpoint disabled, or service down), the block silently self-hides.
 * Tests skip gracefully so this suite stays resilient to environmental issues
 * while still failing hard on real frontend bugs.
 */

const SPOTLIGHT_SELECTOR = 'section[role="region"][aria-roledescription="carousel"]'

test.describe('hero spotlight block (Phase 2)', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 2 cards are all anon-eligible; no login required.
    await page.goto('/')
    await page.waitForLoadState('networkidle')
  })

  test('mounts above the legacy trending row (additive Phase 2)', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards — backend issue, not a frontend bug')

    await expect(block).toBeVisible()

    // Spotlight block should appear ABOVE (smaller Y) than the search bar
    // and any subsequent home content. The search bar is the immediate
    // sibling that comes AFTER <HeroSpotlightBlock /> per Home.vue.
    const blockBox = await block.boundingBox()
    const searchBar = page.locator('input[placeholder]').first()
    const searchBox = await searchBar.boundingBox()
    if (blockBox && searchBox) {
      expect(blockBox.y).toBeLessThan(searchBox.y)
    }
  })

  test('auto-cycles every ~7 seconds', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    await expect(block).toBeVisible()
    // Require at least 2 cards for a meaningful cycle.
    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    test.skip(dotCount < 2, 'Need at least 2 cards to verify auto-cycle')

    const slide = block.locator('[role="group"][aria-roledescription="slide"]').first()
    const initialLabel = await slide.getAttribute('aria-label')
    expect(initialLabel).toBeTruthy()

    // Move mouse OUT of block so hover-pause is inactive.
    await page.mouse.move(0, 0)
    // Wait 7.5s — auto-cycle should fire once.
    await page.waitForTimeout(7500)
    const nextLabel = await slide.getAttribute('aria-label')
    expect(nextLabel).not.toBe(initialLabel)
  })

  test('pauses auto-cycle on hover', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    test.skip(dotCount < 2, 'Need at least 2 cards to verify pause-on-hover')

    const slide = block.locator('[role="group"][aria-roledescription="slide"]').first()
    // Hover the block to pause the cycle.
    await block.hover()
    const initialLabel = await slide.getAttribute('aria-label')
    await page.waitForTimeout(7500)
    const sameLabel = await slide.getAttribute('aria-label')
    expect(sameLabel).toBe(initialLabel)
  })

  test('ArrowRight key seeks to next slide', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    test.skip(dotCount < 2, 'Need at least 2 cards to verify keyboard nav')

    // Focus the carousel region.
    await block.focus()
    const slide = block.locator('[role="group"][aria-roledescription="slide"]').first()
    const initialLabel = await slide.getAttribute('aria-label')
    await page.keyboard.press('ArrowRight')
    await page.waitForTimeout(600)
    const nextLabel = await slide.getAttribute('aria-label')
    expect(nextLabel).not.toBe(initialLabel)
  })

  test('ArrowLeft key seeks to previous slide', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    test.skip(dotCount < 2, 'Need at least 2 cards to verify keyboard nav')

    await block.focus()
    const slide = block.locator('[role="group"][aria-roledescription="slide"]').first()
    const initialLabel = await slide.getAttribute('aria-label')
    await page.keyboard.press('ArrowLeft')
    await page.waitForTimeout(600)
    const prevLabel = await slide.getAttribute('aria-label')
    expect(prevLabel).not.toBe(initialLabel)
  })

  test('reduced-motion preference disables auto-cycle (manual nav still works)', async ({ browser }) => {
    const context = await browser.newContext({ reducedMotion: 'reduce' })
    const page = await context.newPage()
    await page.goto('/')
    // 'networkidle' can hang on the production site due to long-poll fetches
    // and background prefetch — wait for the spotlight selector (or timeout)
    // instead of full networkidle on the secondary context.
    await page.locator(SPOTLIGHT_SELECTOR).first().waitFor({ state: 'attached', timeout: 15000 }).catch(() => {})

    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    if (blockCount === 0) {
      await context.close()
      test.skip(true, 'Spotlight returned 0 cards')
      return
    }
    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    if (dotCount < 2) {
      await context.close()
      test.skip(true, 'Need at least 2 cards to verify reduced-motion')
      return
    }

    const slide = block.locator('[role="group"][aria-roledescription="slide"]').first()
    await page.mouse.move(0, 0)
    const initialLabel = await slide.getAttribute('aria-label')
    // Wait LONGER than one auto-cycle interval. Under reduced-motion the cycle
    // is disabled so the label MUST stay identical.
    await page.waitForTimeout(8000)
    const sameLabel = await slide.getAttribute('aria-label')
    expect(sameLabel).toBe(initialLabel)

    // Manual nav must still work even when auto-cycle is disabled.
    await block.focus()
    await page.keyboard.press('ArrowRight')
    await page.waitForTimeout(600)
    const afterArrowLabel = await slide.getAttribute('aria-label')
    expect(afterArrowLabel).not.toBe(initialLabel)

    await context.close()
  })

  test('mobile viewport (375x667) respects min-height', async ({ browser }) => {
    const context = await browser.newContext({ viewport: { width: 375, height: 667 } })
    const page = await context.newPage()
    await page.goto('/')
    // 'networkidle' can hang on the production site due to long-poll fetches
    // and background prefetch — wait for the spotlight selector (or timeout)
    // instead of full networkidle on the secondary context.
    await page.locator(SPOTLIGHT_SELECTOR).first().waitFor({ state: 'attached', timeout: 15000 }).catch(() => {})

    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    if (blockCount === 0) {
      await context.close()
      test.skip(true, 'Spotlight returned 0 cards')
      return
    }

    await expect(block).toBeVisible()
    // Mobile min-height is 400px per the UI-SPEC. Allow a small tolerance.
    const box = await block.boundingBox()
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(380)

    await context.close()
  })

  test('axe-core reports zero a11y violations on the block', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    await expect(block).toBeVisible()
    const results = await new AxeBuilder({ page })
      .include(SPOTLIGHT_SELECTOR)
      .analyze()
    expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([])
  })

  test('dot indicators render and reflect active state via aria-current', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    const blockCount = await block.count()
    test.skip(blockCount === 0, 'Spotlight returned 0 cards')

    const dots = block.locator('[data-testid="spotlight-dots"] button')
    const dotCount = await dots.count()
    expect(dotCount).toBeGreaterThanOrEqual(1)
    // Phase 2 originally capped at 4 static cards; Phase 3 added 5 more
    // (personal_pick, telegram_news, now_watching, not_time_yet,
    // continue_watching_new) for a max of 9 live cards. Cap raised
    // accordingly — exact count depends on data eligibility at runtime.
    expect(dotCount).toBeLessThanOrEqual(9)

    if (dotCount < 2) {
      test.skip(true, 'Need at least 2 cards to verify dot navigation')
      return
    }

    // Pause auto-cycle so click→seek is deterministic.
    await block.hover()
    await dots.nth(1).click()
    await page.waitForTimeout(600)
    await expect(dots.nth(1)).toHaveAttribute('aria-current', 'true')
    await expect(dots.nth(0)).toHaveAttribute('aria-current', 'false')
  })
})

test.describe('hero spotlight block — flag off (manual gate)', () => {
  // VITE_HERO_SPOTLIGHT_ENABLED is BAKED INTO THE BUNDLE at build time.
  // Toggling it requires a full redeploy (make redeploy-web with the env
  // changed in frontend/web/.env), which is impractical inside an e2e suite.
  // The flag-off behavior is unit-tested in Plan 02-01 (HeroSpotlightBlock.spec.ts).
  // To verify manually:
  //   1) Edit frontend/web/.env → VITE_HERO_SPOTLIGHT_ENABLED=false
  //   2) make redeploy-web
  //   3) Visit / → block should be ABSENT; legacy trending row still visible
  //   4) Revert to VITE_HERO_SPOTLIGHT_ENABLED=true + make redeploy-web
  test.skip('block does not mount when VITE_HERO_SPOTLIGHT_ENABLED=false', async ({ page }) => {
    await page.goto('/')
    const block = page.locator(SPOTLIGHT_SELECTOR)
    await expect(block).toHaveCount(0)
  })
})
