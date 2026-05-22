/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-02 / Task 2,
 * refactored for v1.1-polish Phase 01 Task 5 (HSB-V11-CC-06).
 *
 * Vitest spec for CarouselControls.vue. Verifies:
 *   1. Renders 2 chevron buttons + one labeled-pill dot per card
 *   2. aria-current="true" on active dot, "false" on siblings
 *   3. Click prev/next chevron emits 'prev'/'next' with no payload
 *   4. Click dot N emits 'goto' with 0-indexed payload N
 *   5. Dot aria-label uses the card's kickerKey via t()
 *   6. Dot has a `title` tooltip (same as aria-label)
 *   7. Active dot picks up the card's accent-tinted background
 *   8. No raw text — every label flows through t()
 *
 * `vue-i18n` is stubbed with a fake t() that echoes the key so we can
 * assert against the key directly without loading the actual locale JSON.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { SpotlightCard } from '@/types/spotlight'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import CarouselControls from './CarouselControls.vue'

// Build a minimal SpotlightCard array of `count` cards. Cycles through
// a fixed list of card types so we exercise multiple accent/icon variants.
const ROTATING_TYPES: SpotlightCard['type'][] = [
  'anime_of_day',
  'random_tail',
  'platform_stats',
  'now_watching',
]

function mockCards(count: number): SpotlightCard[] {
  const cards: SpotlightCard[] = []
  for (let i = 0; i < count; i++) {
    const type = ROTATING_TYPES[i % ROTATING_TYPES.length]
    switch (type) {
      case 'anime_of_day':
        cards.push({ type, data: { anime: { id: `aod-${i}` } } })
        break
      case 'random_tail':
        cards.push({ type, data: { anime: { id: `rt-${i}` } } })
        break
      case 'platform_stats':
        cards.push({ type, data: { metrics: [] } })
        break
      case 'now_watching':
        cards.push({ type, data: { sessions: [] } })
        break
    }
  }
  return cards
}

describe('CarouselControls', () => {
  it('renders 2 chevrons + one labeled-pill dot per card', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })

    // 2 chevrons + 4 dots = 6 total focusable buttons
    expect(wrapper.findAll('button').length).toBe(6)
    expect(wrapper.findAll('[data-testid="spotlight-dots"] button').length).toBe(4)
    expect(wrapper.findAll('[aria-current]').length).toBe(4)
  })

  it('marks the active dot with aria-current="true" and others with "false"', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 2, cards: mockCards(4) },
    })

    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(4)
    expect(dots[0].attributes('aria-current')).toBe('false')
    expect(dots[1].attributes('aria-current')).toBe('false')
    expect(dots[2].attributes('aria-current')).toBe('true')
    expect(dots[3].attributes('aria-current')).toBe('false')
  })

  it('emits prev when prev chevron clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(3) },
    })
    const prev = wrapper.find('[aria-label="spotlight.prevSlide"]')
    expect(prev.exists()).toBe(true)
    await prev.trigger('click')
    expect(wrapper.emitted().prev).toBeTruthy()
    expect(wrapper.emitted().prev?.length).toBe(1)
    expect(wrapper.emitted().prev?.[0]).toEqual([])
  })

  it('emits next when next chevron clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(3) },
    })
    const next = wrapper.find('[aria-label="spotlight.nextSlide"]')
    expect(next.exists()).toBe(true)
    await next.trigger('click')
    expect(wrapper.emitted().next).toBeTruthy()
    expect(wrapper.emitted().next?.length).toBe(1)
    expect(wrapper.emitted().next?.[0]).toEqual([])
  })

  it('emits goto with 0-indexed payload when dot clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(4)

    // Click third dot — visually slide 3, internally idx 2 (0-indexed)
    await dots[2].trigger('click')

    const gotoEvents = wrapper.emitted('goto') as unknown as Array<[number]> | undefined
    expect(gotoEvents).toBeTruthy()
    expect(gotoEvents?.length).toBe(1)
    expect(gotoEvents?.[0]?.[0]).toBe(2)
  })

  it('dot aria-label uses the card kickerKey via t()', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')

    // Dot 0 → anime_of_day → spotlight.animeOfDay.title
    expect(dots[0].attributes('aria-label')).toBe('spotlight.animeOfDay.title')
    // Dot 1 → random_tail → spotlight.randomTail.title
    expect(dots[1].attributes('aria-label')).toBe('spotlight.randomTail.title')
    // Dot 2 → platform_stats → spotlight.platformStats.title
    expect(dots[2].attributes('aria-label')).toBe('spotlight.platformStats.title')
    // Dot 3 → now_watching → spotlight.nowWatching.title
    expect(dots[3].attributes('aria-label')).toBe('spotlight.nowWatching.title')
  })

  it('dot title tooltip matches aria-label', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(3) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    for (const dot of dots) {
      expect(dot.attributes('title')).toBe(dot.attributes('aria-label'))
    }
  })

  it('active dot picks up the card accent class (HSB-V11-CC-06)', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 1, cards: mockCards(4) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')

    // Active dot 1 → random_tail → purple accent
    const activeClasses = dots[1].classes().join(' ')
    expect(activeClasses).toMatch(/bg-purple-/)
    expect(activeClasses).toContain('scale-110')

    // Inactive dot 0 → glass-on-glass, no accent
    const inactiveClasses = dots[0].classes().join(' ')
    expect(inactiveClasses).toContain('bg-white/10')
    expect(inactiveClasses).not.toMatch(/bg-purple-/)
  })

  it('each dot renders a SpotlightIcon for its card type', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    // 4 dots × 1 SVG each + 2 chevron SVGs = 6 SVGs total
    expect(wrapper.findAll('svg').length).toBe(6)
  })

  it('has no raw text — every label flows through t()', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 1, cards: mockCards(3) },
    })
    const visibleText = wrapper.text().trim()
    expect(visibleText).not.toMatch(/Previous/i)
    expect(visibleText).not.toMatch(/Next/i)
    expect(visibleText).not.toMatch(/Go to/i)
  })
})
