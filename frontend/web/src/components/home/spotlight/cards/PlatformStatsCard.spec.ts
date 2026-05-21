/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-03 / Task 4.
 *
 * Vitest spec for PlatformStatsCard.vue. Verifies the adaptive 1/2/3-col
 * grid + delta semantics + drift gates.
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

const make = (metrics: Array<{ key: string; value: number; delta?: number | null }>) => ({
  metrics,
})

describe('PlatformStatsCard', () => {
  it('renders 1 metric chip with grid-cols-1 when metrics.length === 1', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 42, delta: 5 }]) },
    })
    const html = wrapper.html()
    expect(html).toContain('grid-cols-1')
    expect(html).not.toContain('md:grid-cols-2')
    expect(html).not.toContain('md:grid-cols-3')
    expect(wrapper.findAll('li').length).toBe(1)
  })

  it('renders 2 chips with md:grid-cols-2 when length === 2', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 10, delta: 1 },
          { key: 'episodes_added_7d', value: 50, delta: 0 },
        ]),
      },
    })
    const html = wrapper.html()
    expect(html).toContain('md:grid-cols-2')
    expect(wrapper.findAll('li').length).toBe(2)
  })

  it('renders 3 chips with md:grid-cols-3 when length === 3', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, delta: 5 },
          { key: 'episodes_added_7d', value: 1024, delta: 0 },
          { key: 'active_rooms_7d', value: 7, delta: null },
        ]),
      },
    })
    const html = wrapper.html()
    expect(html).toContain('md:grid-cols-3')
    expect(wrapper.findAll('li').length).toBe(3)
  })

  it('renders text-cyan-400 deltaPositive when delta > 0', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 42, delta: 5 }]) },
    })
    const html = wrapper.html()
    expect(html).toContain('text-cyan-400')
    // The positive-delta paragraph carries the i18n key with the delta value.
    expect(wrapper.text()).toContain('spotlight.platformStats.deltaPositive')
    expect(wrapper.text()).toContain('"n":5')
  })

  it('renders text-gray-500 noChange when delta === 0 or null', () => {
    const wrapperZero = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 10, delta: 0 }]) },
    })
    expect(wrapperZero.html()).toContain('text-gray-500')
    expect(wrapperZero.text()).toContain('spotlight.platformStats.noChange')

    const wrapperNull = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 10, delta: null }]) },
    })
    expect(wrapperNull.html()).toContain('text-gray-500')
    expect(wrapperNull.text()).toContain('spotlight.platformStats.noChange')
  })

  it('uses tabular-nums on value paragraph', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 1234, delta: 5 }]) },
    })
    expect(wrapper.html()).toContain('tabular-nums')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 1, delta: 1 }]) },
    })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: { data: make([{ key: 'anime_added_7d', value: 1, delta: 1 }]) },
    })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })

  it('has no hardcoded English text — all labels via t()', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, delta: 5 },
          { key: 'episodes_added_7d', value: 1024, delta: 0 },
        ]),
      },
    })
    const text = wrapper.text()
    expect(text).toContain('spotlight.platformStats.title')
    expect(text).not.toMatch(/Platform this week/)
    expect(text).not.toMatch(/vs last week/)
  })

  it('uses camelCase i18n keys for metric labels (animeAdded7d, etc.)', () => {
    const wrapper = mount(PlatformStatsCard, {
      props: {
        data: make([
          { key: 'anime_added_7d', value: 42, delta: 5 },
          { key: 'episodes_added_7d', value: 1024, delta: 1 },
          { key: 'active_rooms_7d', value: 7, delta: null },
        ]),
      },
    })
    const text = wrapper.text()
    // Per UI-SPEC Copywriting Contract, i18n keys are camelCase (animeAdded7d)
    // not snake_case (anime_added_7d). The card converts the backend's
    // snake_case `m.key` for label lookup.
    expect(text).toContain('spotlight.platformStats.animeAdded7d')
    expect(text).toContain('spotlight.platformStats.episodesAdded7d')
    expect(text).toContain('spotlight.platformStats.activeRooms7d')
  })
})
