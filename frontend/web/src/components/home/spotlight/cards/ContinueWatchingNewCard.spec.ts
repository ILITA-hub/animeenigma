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

  it('renders the hero ribbon spanning the full top of the poster', () => {
    const wrapper = mountCard({ data: baseData })
    const ribbon = wrapper
      .findAll('div')
      .find(
        (d) =>
          d.classes().includes('inset-x-0') && d.classes().includes('top-0'),
      )
    expect(ribbon).toBeDefined()
    // Ribbon carries the purple->fuchsia gradient identity.
    expect(ribbon!.classes()).toContain('from-purple-600')
    expect(ribbon!.classes()).toContain('to-fuchsia-500')
  })

  it('ribbon text interpolates new_episode_number via newEpisodeBadge', () => {
    const wrapper = mountCard({ data: baseData })
    const ribbon = wrapper
      .findAll('div')
      .find(
        (d) =>
          d.classes().includes('inset-x-0') && d.classes().includes('top-0'),
      )
    expect(ribbon!.text()).toContain('newEpisodeBadge')
    expect(ribbon!.text()).toContain('"n":7')
  })

  it('renders BOTH the last-watched line and the new-episode line (count = 2)', () => {
    const wrapper = mountCard({ data: baseData })
    const text = wrapper.text()
    // Subdued "watched up to" line interpolates last_watched_episode (5).
    expect(text).toContain('spotlight.continueWatchingNew.lastWatched::{"n":5}')
    // Accent "new episode" line interpolates new_episode_number (7).
    expect(text).toContain('spotlight.continueWatchingNew.newEpisodeLine::{"n":7}')
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

  it('backdrop secondary overlay carries the from-purple-500/30 wash', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.html()).toContain('from-purple-500/30')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })
})
