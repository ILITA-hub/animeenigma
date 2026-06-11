/**
 * Workstream hero-spotlight — Vitest spec for CarouselDots.vue.
 *
 * The labeled-pill dot indicators were split out of CarouselControls.vue in
 * the v1.1-polish crop fix (dots now render BELOW the frame). Verifies:
 *   1. Renders one labeled-pill dot per card
 *   2. aria-current="true" on active dot, "false" on siblings
 *   3. Click dot N emits 'goto' with 0-indexed payload N
 *   4. Dot aria-label uses the card's kickerKey via t()
 *   5. Dot has a `title` tooltip (same as aria-label)
 *   6. Active dot picks up the card's accent-tinted background
 *   7. Each dot renders a SpotlightIcon for its card type
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
import CarouselDots from './CarouselDots.vue'

// Build a minimal SpotlightCard array of `count` cards. Cycles through
// a fixed list of card types so we exercise multiple accent/icon variants.
const ROTATING_TYPES: SpotlightCard['type'][] = [
  'featured',
  'random_tail',
  'platform_stats',
  'now_watching',
]

function mockCards(count: number): SpotlightCard[] {
  const cards: SpotlightCard[] = []
  for (let i = 0; i < count; i++) {
    const type = ROTATING_TYPES[i % ROTATING_TYPES.length]
    switch (type) {
      case 'featured':
        cards.push({ type, data: { anime: { id: `feat-${i}` } } })
        break
      case 'random_tail':
        cards.push({ type, data: { anime: { id: `rt-${i}` } } })
        break
      case 'platform_stats':
        cards.push({
          type,
          data: {
            hero: {
              working_ok: true,
              uptime_quip: 'ОЧЕНЬ МНОГО',
              service: 'catalog',
              ux_delta: '+5',
              cdi: '0.00 * 99',
              mvq: 'Dragon 99%/99%',
              tagline: 'Лучшая платформа.',
            },
            tiles: [],
          },
        })
        break
      case 'now_watching':
        cards.push({ type, data: { sessions: [] } })
        break
    }
  }
  return cards
}

describe('CarouselDots', () => {
  it('renders one labeled-pill dot per card', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    expect(wrapper.findAll('[data-testid="spotlight-dots"] button').length).toBe(4)
    expect(wrapper.findAll('[aria-current]').length).toBe(4)
  })

  it('marks the active dot with aria-current="true" and others with "false"', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 2, cards: mockCards(4) },
    })

    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(4)
    expect(dots[0].attributes('aria-current')).toBe('false')
    expect(dots[1].attributes('aria-current')).toBe('false')
    expect(dots[2].attributes('aria-current')).toBe('true')
    expect(dots[3].attributes('aria-current')).toBe('false')
  })

  it('emits goto with 0-indexed payload when dot clicked', async () => {
    const wrapper = mount(CarouselDots, {
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
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')

    // Dot 0 → featured → spotlight.featured.title
    expect(dots[0].attributes('aria-label')).toBe('spotlight.featured.title')
    // Dot 1 → random_tail → spotlight.randomTail.title
    expect(dots[1].attributes('aria-label')).toBe('spotlight.randomTail.title')
    // Dot 2 → platform_stats → spotlight.platformStats.title
    expect(dots[2].attributes('aria-label')).toBe('spotlight.platformStats.title')
    // Dot 3 → now_watching → spotlight.nowWatching.title
    expect(dots[3].attributes('aria-label')).toBe('spotlight.nowWatching.title')
  })

  it('dot title tooltip matches aria-label', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 0, cards: mockCards(3) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    for (const dot of dots) {
      expect(dot.attributes('title')).toBe(dot.attributes('aria-label'))
    }
  })

  it('active dot picks up the card accent class (HSB-V11-CC-06)', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 1, cards: mockCards(4) },
    })
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')

    // Active item 1 → random_tail → violet accent pill (v4 A-1 icon menu)
    const activeClasses = dots[1].classes().join(' ')
    expect(activeClasses).toMatch(/bg-brand-violet\/20/)
    expect(activeClasses).toContain('menu-active')

    // Inactive dot 0 → glass-on-glass, no accent
    const inactiveClasses = dots[0].classes().join(' ')
    expect(inactiveClasses).toContain('bg-white/10')
    expect(inactiveClasses).not.toMatch(/bg-purple-/)
  })

  it('each dot renders a SpotlightIcon for its card type', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 0, cards: mockCards(4) },
    })
    // 4 dots × 1 SVG each
    expect(wrapper.findAll('svg').length).toBe(4)
  })

  it('renders exactly one dot when given a single card', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 0, cards: mockCards(1) },
    })
    expect(wrapper.findAll('[data-testid="spotlight-dots"] button').length).toBe(1)
  })

  it('has no raw text — every label flows through t()', () => {
    const wrapper = mount(CarouselDots, {
      props: { currentIndex: 1, cards: mockCards(3) },
    })
    const visibleText = wrapper.text().trim()
    expect(visibleText).not.toMatch(/Go to/i)
  })
})
