/**
 * Workstream hero-spotlight — Phase 3 (dynamic-cards-migration) Plan 03-05 / Task 3.
 *
 * Vitest spec for NotTimeYetCard.vue.
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

import NotTimeYetCard from './NotTimeYetCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(NotTimeYetCard, {
    props: props as unknown as InstanceType<typeof NotTimeYetCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const baseData = {
  status: 'planned' as const,
  anime: {
    id: 'a-1',
    name: 'Frieren',
    name_ru: 'Фрирен',
    poster_url: '/poster.jpg',
  },
}

describe('NotTimeYetCard', () => {
  it('renders the title key from i18n', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.title')
  })

  it('renders subtitlePlanned when status=planned', () => {
    const wrapper = mountCard({ data: { ...baseData, status: 'planned' } })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.subtitlePlanned')
    expect(wrapper.text()).not.toContain('spotlight.notTimeYet.subtitlePostponed')
  })

  it('renders subtitlePostponed when status=postponed', () => {
    const wrapper = mountCard({ data: { ...baseData, status: 'postponed' } })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.subtitlePostponed')
    expect(wrapper.text()).not.toContain('spotlight.notTimeYet.subtitlePlanned')
  })

  it('CTA links to /anime/{id}', () => {
    const wrapper = mountCard({ data: baseData })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const cta = links.find((l) => l.props('to') === '/anime/a-1')
    expect(cta).toBeDefined()
  })

  it('renders poster with anime.poster_url', () => {
    const wrapper = mountCard({ data: baseData })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('/poster.jpg')
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
