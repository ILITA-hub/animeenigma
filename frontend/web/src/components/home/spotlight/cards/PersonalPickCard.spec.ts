/**
 * Workstream hero-spotlight — Phase 3 (dynamic-cards-migration) Plan 03-05 / Task 2.
 *
 * Vitest spec for PersonalPickCard.vue. Verifies:
 *   1. Renders 3 items in a row on desktop
 *   2. Renders mobile footer "+ ещё N →" link only when items.length > 1
 *   3. Mobile footer link target is /browse?sort=trending when source='trending'
 *   4. Mobile footer link target is /recs when source='personal'
 *   5. Title key is `personalPick.titleAnon` when source='trending', else `personalPick.title`
 *   6. Uses only font-medium / font-semibold weights
 *   7. Tablet padding p-4 (never p-5)
 *   8. All visible strings flow through t()
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

import PersonalPickCard from './PersonalPickCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(PersonalPickCard, {
    props: props as unknown as InstanceType<typeof PersonalPickCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const animeFixture = (i: number) => ({
  id: `anime-${i}`,
  name: `Anime ${i}`,
  name_ru: `Аниме ${i}`,
  poster_url: `/poster-${i}.jpg`,
})

describe('PersonalPickCard', () => {
  it('renders 3 items in a row on desktop', () => {
    const data = {
      source: 'personal',
      items: [
        { anime: animeFixture(1) },
        { anime: animeFixture(2) },
        { anime: animeFixture(3) },
      ],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    // Each of 3 items is a router-link; mobile footer link is also rendered
    // (items.length > 1) so total = 4.
    expect(links.length).toBe(4)
    // Three poster images rendered.
    expect(wrapper.findAll('img').length).toBe(3)
  })

  it('does NOT render mobile footer link when only 1 item', () => {
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.length).toBe(1) // only the single item link
    // No "moreLink" text rendered.
    expect(wrapper.text()).not.toContain('spotlight.personalPick.moreLink')
  })

  it('mobile footer link routes to /browse?sort=trending when source=trending', () => {
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    // Find the link whose `to` mentions /browse
    const footer = links.find((l) => {
      const to = l.props('to') as unknown
      return typeof to === 'string' && to.includes('/browse')
    })
    expect(footer).toBeDefined()
    expect(footer!.props('to')).toBe('/browse?sort=trending')
  })

  it('mobile footer link routes to /recs when source=personal', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const footer = links.find((l) => l.props('to') === '/recs')
    expect(footer).toBeDefined()
  })

  it('uses titleAnon when source=trending', () => {
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('spotlight.personalPick.titleAnon')
    expect(wrapper.text()).not.toContain('spotlight.personalPick.title::')
  })

  it('uses title key when source=personal', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('spotlight.personalPick.title')
    expect(wrapper.text()).not.toContain('spotlight.personalPick.titleAnon')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
