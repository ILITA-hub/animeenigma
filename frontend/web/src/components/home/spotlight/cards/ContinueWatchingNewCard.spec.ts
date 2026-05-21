/**
 * Workstream hero-spotlight — Phase 3 (dynamic-cards-migration) Plan 03-05 / Task 3.
 *
 * Vitest spec for ContinueWatchingNewCard.vue.
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

import ContinueWatchingNewCard from './ContinueWatchingNewCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(ContinueWatchingNewCard, {
    props: props as unknown as InstanceType<typeof ContinueWatchingNewCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const baseData = {
  anime: {
    id: 'a-7',
    name: 'Frieren',
    name_ru: 'Фрирен',
    poster_url: '/poster.jpg',
  },
  last_watched_episode: 5,
  new_episode_number: 7,
}

describe('ContinueWatchingNewCard', () => {
  it('renders the title key from i18n', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.continueWatchingNew.title')
  })

  it('renders new-episode badge with interpolated n', () => {
    const wrapper = mountCard({ data: baseData })
    // t() mock JSON-encodes params; assert the new_episode_number flows through.
    expect(wrapper.text()).toContain('"n":7')
  })

  it('renders last_watched_episode somewhere visible', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('5')
  })

  it('CTA links to /anime/{id}', () => {
    const wrapper = mountCard({ data: baseData })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const cta = links.find((l) => {
      const to = l.props('to')
      return typeof to === 'string' && to.includes('/anime/a-7')
    })
    expect(cta).toBeDefined()
  })

  it('new-episode badge carries purple visual styling', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.html()).toContain('bg-purple-500')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
