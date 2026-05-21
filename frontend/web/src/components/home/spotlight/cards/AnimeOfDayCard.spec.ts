/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-03 / Task 1.
 *
 * Vitest spec for AnimeOfDayCard.vue. Verifies:
 *   1. Renders localized title via getLocalizedTitle
 *   2. Renders score chip (yellow-400) when score truthy
 *   3. Omits score chip when score undefined
 *   4. Caps genres at 3 (slice(0, 3))
 *   5. Renders Watch (btn-primary) and Add (btn-ghost) CTAs
 *   6. Uses only font-medium / font-semibold typography weights
 *   7. Tablet padding p-4 — never p-5
 *   8. Has no hardcoded English strings — every label flows through t()
 *
 * `vue-i18n` is stubbed with a t() that echoes the key (plus JSON-encoded
 * params if present); `@/utils/title` is stubbed to a first-non-empty
 * passthrough. router-link is satisfied via RouterLinkStub.
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

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import AnimeOfDayCard from './AnimeOfDayCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(AnimeOfDayCard, {
    props,
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const baseMockData = {
  anime: {
    id: 'abc-123',
    name: 'Frieren',
    name_ru: 'Фрирен',
    poster_url: '/test-poster.jpg',
    score: 9.1,
    episodes_count: 28,
    genres: [
      { id: '1', name: 'Adventure', russian: 'Приключения' },
      { id: '2', name: 'Drama', russian: 'Драма' },
    ],
  },
}

describe('AnimeOfDayCard', () => {
  it('renders the localized title', () => {
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.text()).toContain('Frieren')
  })

  it('renders score chip with yellow-400 when score is truthy', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    expect(html).toContain('text-yellow-400')
    expect(wrapper.text()).toContain('9.1')
  })

  it('does not render score chip when score is undefined', () => {
    const data = {
      anime: { ...baseMockData.anime, score: undefined },
    }
    const wrapper = mountCard({ data })
    // The yellow-400 score chip should be absent. Other yellow may exist
    // nowhere in this card per UI-SPEC — so a string check is sufficient.
    expect(wrapper.html()).not.toContain('text-yellow-400')
  })

  it('caps genres at 3', () => {
    const data = {
      anime: {
        ...baseMockData.anime,
        genres: [
          { id: '1', name: 'A', russian: 'А' },
          { id: '2', name: 'B', russian: 'Б' },
          { id: '3', name: 'C', russian: 'В' },
          { id: '4', name: 'D', russian: 'Г' },
          { id: '5', name: 'E', russian: 'Д' },
        ],
      },
    }
    const wrapper = mountCard({ data })
    // Genre chips use the exact class string from UI-SPEC.
    const chips = wrapper.findAll('span.bg-white\\/10')
    expect(chips.length).toBe(3)
  })

  it('renders Watch (btn-primary) and Add (btn-ghost) CTAs', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    expect(html).toContain('btn-primary')
    expect(html).toContain('btn-ghost')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })

  it('has no hardcoded English strings — all labels via t()', () => {
    const wrapper = mountCard({ data: baseMockData })
    const text = wrapper.text()
    // With t() stubbed to echo keys, the i18n keys appear as visible text.
    expect(text).toContain('spotlight.animeOfDay.watchCta')
    expect(text).toContain('spotlight.animeOfDay.addCta')
    // Raw English copy should NOT leak — only the keys.
    expect(text).not.toMatch(/\bWatch\b/)
    expect(text).not.toMatch(/Add to list/)
  })
})
