import { test, expect, type Page } from '@playwright/test'

/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 Task 6 (HSB-V11-CC-05).
 *
 * Regression test for the Phase 03 UAT "blank-card" finding: hammering
 * ArrowRight 10× rapidly used to outrace the 400ms cross-fade and leave
 * the carousel stuck in the leave-to opacity:0 state with no card visible.
 *
 * Phase 01 Task 4 added an `isTransitioning` lock to HeroSpotlightBlock.vue
 * (gated next/prev/goTo + 600ms watchdog) — this spec proves the lock
 * holds under 10 rapid presses.
 *
 * Strategy: mock the 9-card payload so the carousel deterministically has
 * exactly 9 cards. Focus the carousel region. Fire 10 rapid ArrowRight
 * presses. Wait one watchdog window. Assert:
 *
 *   1. The active card type ended up different from the initial card type
 *      (i.e. presses did advance, they weren't ALL no-ops).
 *   2. No element is stuck mid-fade (.spotlight-fade-leave-active count
 *      is 0). This is the actual blank-card signal.
 *
 * Mirrors the mocking pattern used in e2e/spotlight-full.spec.ts so the
 * payload is identical and any 9-card-specific bugs surface in both specs.
 */

const SPOTLIGHT_SELECTOR = 'section[role="region"][aria-roledescription="carousel"]'

const nineCardPayload = {
  cards: [
    { type: 'anime_of_day', data: { anime: { id: 'a1', name: 'AnimeOfDay Fixture', poster_url: 'x' } } },
    { type: 'random_tail', data: { anime: { id: 'a2', name: 'RandomTail Fixture', poster_url: 'x' } } },
    {
      type: 'latest_news',
      data: {
        entries: [
          { date: '2026-05-21', type: 'feature', message: 'Test changelog entry' },
        ],
      },
    },
    { type: 'platform_stats', data: { metrics: [{ key: 'anime_added_7d', value: 12 }] } },
    {
      type: 'personal_pick',
      data: {
        items: [{ anime: { id: 'p1', name: 'PersonalPick Fixture', poster_url: 'x' } }],
        source: 'trending',
      },
    },
    {
      type: 'telegram_news',
      data: {
        posts: [
          { excerpt: 'tg-one' },
          { excerpt: 'tg-two' },
          { excerpt: 'tg-three' },
        ],
      },
    },
    {
      type: 'now_watching',
      data: {
        sessions: [
          {
            username: 'fixture_user',
            public_id: 'fixture-user',
            anime_id: 'nw1',
            anime_name: 'NowWatching Fixture',
            episode_number: 3,
            updated_at: '2026-05-21T18:00:00Z',
          },
        ],
      },
    },
    {
      type: 'not_time_yet',
      data: {
        anime: { id: 'nty1', name: 'NotTimeYet Fixture', poster_url: 'x' },
        status: 'planned',
      },
    },
    {
      type: 'continue_watching_new',
      data: {
        anime: { id: 'cwn1', name: 'ContinueWatchingNew Fixture', poster_url: 'x' },
        last_watched_episode: 5,
        new_episode_number: 7,
      },
    },
  ],
  generated_at: '2026-05-21T18:00:00Z',
}

async function mockSpotlight(page: Page) {
  await page.route('**/api/home/spotlight**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(nineCardPayload),
    }),
  )
}

async function forceEnglishLocale(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('locale', 'en')
  })
}

// Returns the slide group's aria-label, which encodes the card's title
// via the i18n cardTitle() resolver in HeroSpotlightBlock.vue. Two cards
// of the same type would share a label — but the 9-card mock has all 9
// types distinct so the label uniquely identifies the active card.
async function activeSlideLabel(page: Page): Promise<string> {
  const slide = page
    .locator(SPOTLIGHT_SELECTOR)
    .locator('[role="group"][aria-roledescription="slide"]')
    .first()
  return (await slide.getAttribute('aria-label')) ?? ''
}

test.describe('hero spotlight transition lock (v1.1-polish HSB-V11-CC-05)', () => {
  test.beforeEach(async ({ page }) => {
    await mockSpotlight(page)
    await forceEnglishLocale(page)
    await page.goto('/')
    await page
      .locator(SPOTLIGHT_SELECTOR)
      .first()
      .waitFor({ state: 'visible', timeout: 15000 })
  })

  test('10 rapid ArrowRight presses settle without leaving a card stuck mid-fade', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    await expect(block).toBeVisible()

    // Focus the region so keyboard events route to it (HSB-FE-06 navigation).
    await block.focus()

    const initialLabel = await activeSlideLabel(page)
    expect(initialLabel).not.toBe('')

    // Fire 10 ArrowRight presses as fast as Playwright will let us. The lock
    // gates next() while a fade is in flight (~400ms), so the bulk of these
    // presses become no-ops — but the user MUST be able to land on a stable
    // final card and see no opacity:0 ghost.
    for (let i = 0; i < 10; i++) {
      await page.keyboard.press('ArrowRight')
    }

    // One full watchdog window (600ms) + transition buffer (200ms) so the
    // last accepted press has time to finish its cross-fade before assertions.
    await page.waitForTimeout(900)

    // Critical assertion #1: no element is stuck in the leave phase.
    // .spotlight-fade-leave-active is the CSS class Vue applies while the
    // outgoing card is fading out; it should never be present after the
    // carousel settles (Phase 03 UAT bug signature).
    const stuckLeave = await page.locator('.spotlight-fade-leave-active').count()
    expect(stuckLeave).toBe(0)

    // Critical assertion #2: the carousel actually advanced (the lock didn't
    // swallow ALL 10 presses). With 9 distinct card types and the locked
    // cadence of ~400ms per fade, at minimum 1 advance must succeed.
    const finalLabel = await activeSlideLabel(page)
    expect(finalLabel).not.toBe(initialLabel)

    // Sanity: the carousel region is still visible (no DOM rip).
    await expect(block).toBeVisible()
  })

  test('lock honors goTo: rapid dot clicks do not produce stuck states', async ({ page }) => {
    const block = page.locator(SPOTLIGHT_SELECTOR)
    await expect(block).toBeVisible()
    await block.hover() // pause auto-cycle to keep clicks deterministic

    const dots = block.locator('[data-testid="spotlight-dots"] button')
    await expect(dots).toHaveCount(9)

    // Hammer 5 different dots in rapid succession.
    await dots.nth(2).click()
    await dots.nth(5).click()
    await dots.nth(8).click()
    await dots.nth(1).click()
    await dots.nth(4).click()

    await page.waitForTimeout(900)

    const stuckLeave = await page.locator('.spotlight-fade-leave-active').count()
    expect(stuckLeave).toBe(0)
    await expect(block).toBeVisible()
  })
})
