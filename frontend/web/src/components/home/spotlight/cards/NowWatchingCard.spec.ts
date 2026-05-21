/**
 * Workstream hero-spotlight — Phase 3 (dynamic-cards-migration) Plan 03-05 / Task 2.
 *
 * Vitest spec for NowWatchingCard.vue.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

import NowWatchingCard from './NowWatchingCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(NowWatchingCard, {
    props: props as unknown as InstanceType<typeof NowWatchingCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const session = (i: number, overrides: Record<string, unknown> = {}) => ({
  username: `u${i}`,
  public_id: `pid-${i}`,
  anime_id: `anime-${i}`,
  anime_name: `AnimeName${i}`,
  anime_name_ru: `Аниме${i}`,
  poster_url: `/poster-${i}.jpg`,
  episode_number: i + 4,
  updated_at: '2026-05-21T10:00:00Z',
  ...overrides,
})

describe('NowWatchingCard', () => {
  it.each([1, 2, 3])('renders %i rows for %i sessions', (n) => {
    const data = {
      sessions: Array.from({ length: n }, (_, i) => session(i + 1)),
    }
    const wrapper = mountCard({ data })
    const rows = wrapper.findAllComponents(RouterLinkStub)
    expect(rows.length).toBe(n)
  })

  it('renders a live green dot on every row', () => {
    const data = { sessions: [session(1), session(2), session(3)] }
    const wrapper = mountCard({ data })
    const dots = wrapper.findAll('span.bg-green-400')
    expect(dots.length).toBe(3)
  })

  it('session row label contains username, anime, and episode number', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    // t() mock JSON-encodes params; assert all three are interpolated.
    const text = wrapper.text()
    expect(text).toContain('"username":"u1"')
    expect(text).toContain('"anime":"AnimeName1"')
    expect(text).toContain('"n":5')
  })

  it('links each row to /anime/{id}', () => {
    const data = { sessions: [session(1), session(2)] }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links[0].props('to')).toBe('/anime/anime-1')
    expect(links[1].props('to')).toBe('/anime/anime-2')
  })

  it('omits avatar img when poster_url missing', () => {
    const data = { sessions: [session(1, { poster_url: undefined })] }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('img').length).toBe(0)
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
