/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-03 / Task 2.
 *
 * Vitest spec for RandomTailCard.vue. Verifies the visual deltas vs
 * AnimeOfDayCard:
 *   1. Eyebrow uses text-cyan-300/80 (dimmer cyan) — NOT text-cyan-400
 *   2. Desktop subtitle is hidden on mobile (hidden md:block)
 *   3. Single Open CTA (btn-primary); NO Add (btn-ghost)
 *   4. Renders localized title via getLocalizedTitle
 *   5. Only font-medium / font-semibold weights
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

import RandomTailCard from './RandomTailCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(RandomTailCard, {
    props,
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const mockData = {
  anime: {
    id: 'xyz-456',
    name: 'Mushishi',
    name_ru: 'Мусиси',
    poster_url: '/test-poster.jpg',
    score: 8.6,
    episodes_count: 26,
  },
}

describe('RandomTailCard', () => {
  it('renders the RandomTail eyebrow with text-cyan-300/80 (NOT text-cyan-400)', () => {
    const wrapper = mountCard({ data: mockData })
    const html = wrapper.html()
    expect(html).toContain('text-cyan-300/80')
    // The dimmer cyan is the visual differentiator from AnimeOfDayCard.
    // The score chip is yellow (text-yellow-400) and the discover CTA does
    // NOT use cyan-400 text — so a bare absence-of-text-cyan-400 assertion
    // is safe.
    expect(html).not.toContain('text-cyan-400')
  })

  it('renders the desktop subtitle hidden on mobile', () => {
    const wrapper = mountCard({ data: mockData })
    const html = wrapper.html()
    // The subtitle key is present somewhere…
    expect(wrapper.text()).toContain('spotlight.randomTail.subtitle')
    // …and the hidden md:block utility pair is used (subtitle + desktop eyebrow).
    expect(html).toContain('hidden md:block')
    // The subtitle paragraph specifically carries the hidden+md:block pair.
    // Find the paragraph by searching for the subtitle key inside an element
    // whose class string includes `hidden md:block`.
    const subtitlePara = wrapper.findAll('p').find((p) =>
      p.text().includes('spotlight.randomTail.subtitle'),
    )
    expect(subtitlePara).toBeTruthy()
    expect(subtitlePara?.classes()).toContain('hidden')
    expect(subtitlePara?.classes()).toContain('md:block')
  })

  it('renders a single Open CTA (btn-primary) and NO Add CTA (btn-ghost)', () => {
    const wrapper = mountCard({ data: mockData })
    const html = wrapper.html()
    // exactly one btn-primary occurrence
    const primaryMatches = html.match(/btn-primary/g) ?? []
    expect(primaryMatches.length).toBe(1)
    expect(html).not.toContain('btn-ghost')
    // No add CTA key — neither the animeOfDay one nor any hypothetical
    // randomTail.addCta.
    expect(wrapper.text()).not.toContain('spotlight.animeOfDay.addCta')
    expect(wrapper.text()).not.toContain('spotlight.randomTail.addCta')
    // The lone CTA is the discover one.
    expect(wrapper.text()).toContain('spotlight.randomTail.discoverCta')
  })

  it('renders the localized title via getLocalizedTitle', () => {
    const wrapper = mountCard({ data: mockData })
    expect(wrapper.text()).toContain('Mushishi')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: mockData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const wrapper = mountCard({ data: mockData })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
