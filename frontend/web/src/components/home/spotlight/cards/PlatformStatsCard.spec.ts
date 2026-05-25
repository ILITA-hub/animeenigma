/**
 * Workstream hero-spotlight — v1.1-polish Phase 08 (platform-stats-refactor).
 *
 * Vitest spec for the refactored PlatformStatsCard.vue: hero stat (text-7xl/8xl)
 * + sparkline + delta chip + 2×2 supporting micro-grid. Replaces the Phase-2
 * adaptive-grid spec (the 1/2/3-col layout was removed in this refactor).
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import PlatformStatsCard from './PlatformStatsCard.vue'

interface Metric {
  key: string
  value: number
  previous_value?: number | null
  series?: number[]
  delta?: number | null
}
const make = (metrics: Metric[]) => ({ metrics })

const SERIES = [1, 2, 3, 4, 5, 6, 7]

describe('PlatformStatsCard (refactored)', () => {
  it('renders the hero stat at text-7xl / text-8xl', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: SERIES },
        ]),
      },
    })
    const html = wrapper.html()
    expect(html).toContain('text-7xl')
    expect(html).toContain('md:text-8xl')
    // The hero value renders.
    expect(wrapper.text()).toContain('42')
  })

  it('renders a Sparkline whose data-points match series.join(",")', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: SERIES },
        ]),
      },
    })
    const svg = wrapper.find('svg[data-points]')
    expect(svg.exists()).toBe(true)
    expect(svg.attributes('data-points')).toBe(SERIES.join(','))
  })

  it('omits the Sparkline when series has fewer than 2 points', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: [5] },
        ]),
      },
    })
    expect(wrapper.find('svg[data-points]').exists()).toBe(false)
  })

  it('DeltaChip shows ↑ when value > previous_value', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 120, previous_value: 100, series: SERIES },
        ]),
      },
    })
    expect(wrapper.text()).toContain('↑')
  })

  it('DeltaChip shows ↓ when value < previous_value', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 50, previous_value: 100, series: SERIES },
        ]),
      },
    })
    expect(wrapper.text()).toContain('↓')
  })

  it('renders exactly 4 supporting metrics when 5 metrics provided (hero + 4)', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 1, previous_value: 1, series: SERIES },
          { key: 'episodes_added_7d', value: 2, previous_value: 1 },
          { key: 'active_rooms_7d', value: 3, previous_value: 1 },
          { key: 'm4', value: 4, previous_value: 1 },
          { key: 'm5', value: 5, previous_value: 1 },
          { key: 'm6_ignored', value: 6, previous_value: 1 },
        ]),
      },
    })
    // 4 supporting <li> (slice(1,5) drops hero[0] and the 6th metric).
    expect(wrapper.findAll('li').length).toBe(4)
  })

  it('renders no supporting <li> when only the hero metric is present', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: SERIES },
        ]),
      },
    })
    expect(wrapper.findAll('li').length).toBe(0)
  })

  it('is a single-root <article> (Transition out-in safety)', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: SERIES },
        ]),
      },
    })
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('uses only font-medium / font-semibold weights and p-4 padding base', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 1, previous_value: 1, series: SERIES },
        ]),
      },
    })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
    expect(html).toContain('p-4')
  })

  it('labels metrics via camelCase i18n keys (no hardcoded English)', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, previous_value: 30, series: SERIES },
          { key: 'episodes_added_7d', value: 10, previous_value: 5 },
        ]),
      },
    })
    const text = wrapper.text()
    expect(text).toContain('spotlight.platformStats.title')
    expect(text).toContain('spotlight.platformStats.animeAdded7d')
    expect(text).toContain('spotlight.platformStats.episodesAdded7d')
    expect(text).toContain('spotlight.platformStats.vsPriorWeek')
    expect(text).not.toMatch(/Platform this week/)
  })

  it('uses tabular-nums on the numeric stat paragraphs', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 1234, previous_value: 1000, series: SERIES },
        ]),
      },
    })
    expect(wrapper.html()).toContain('tabular-nums')
  })
})
