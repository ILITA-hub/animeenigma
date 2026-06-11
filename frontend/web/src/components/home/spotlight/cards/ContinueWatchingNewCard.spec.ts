/**
 * Workstream hero-spotlight — v1.1-polish Phase 10
 * (continue-watching-new-refactor) Plan 10 / Task 5.
 *
 * Vitest spec for the hero-ribbon + deep-link refactor of
 * ContinueWatchingNewCard.vue.
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
  it('renders a single <article> root', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('renders the title key from i18n', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.continueWatchingNew.title')
  })

  it('renders the episode stepper with the NEW chip (v4 H-4 — ribbon removed)', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.find('[data-testid="episode-stepper"]').exists()).toBe(true)
    const newChip = wrapper.find('[data-testid="stepper-new"]')
    expect(newChip.exists()).toBe(true)
    expect(newChip.classes().join(' ')).toContain('text-pink-400')
  })

  it('stepper NEW chip interpolates new_episode_number via epChipNew (v4 H-4)', () => {
    const wrapper = mountCard({ data: baseData })
    const newChip = wrapper.find('[data-testid="stepper-new"]')
    expect(newChip.text()).toContain('spotlight.continueWatchingNew.epChipNew')
    expect(newChip.text()).toContain(String(baseData.new_episode_number))
  })

  it('renders the new-episode meta line + watched chips (v4 H-4)', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.continueWatchingNew.newEpisodeLine')
    // last_watched_episode renders as a dimmed ✓ chip inside the stepper
    expect(wrapper.text()).toContain('spotlight.continueWatchingNew.epChip')
  })

  it('CTA deep-links to /anime/{id}?episode={n} (canonical, no /watch hop)', () => {
    const wrapper = mountCard({ data: baseData })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const cta = links.find((l) => {
      const to = l.props('to')
      return typeof to === 'string' && to.endsWith('?episode=7')
    })
    expect(cta).toBeDefined()
    expect(cta!.props('to')).toBe('/anime/a-7?episode=7')
    // No legacy /watch redirect hop in the href.
    expect(cta!.props('to')).not.toContain('/watch?episode=')
  })

  it('CTA episode matches data.new_episode_number', () => {
    const wrapper = mountCard({ data: baseData })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const cta = links.find((l) => {
      const to = l.props('to')
      return typeof to === 'string' && to.endsWith(`?episode=${baseData.new_episode_number}`)
    })
    expect(cta).toBeDefined()
  })

  it('backdrop secondary overlay carries the from-pink-500/25 wash', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.html()).toContain('from-pink-500/25')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })
})
