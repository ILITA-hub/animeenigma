/**
 * Workstream hero-spotlight — v1.1-polish Phase 02 (HSB-V11-AOD-01..04).
 *
 * Vitest spec for AnimeOfDayCard.vue (refactored). Verifies:
 *   1. Renders localized title via getLocalizedTitle
 *   2. Renders score badge (yellow-500/20) as a meta-row pill (not absolute)
 *   3. Omits score pill when score undefined
 *   4. Caps genres at 3 (slice(0, 3))
 *   5. Renders the cinematic SpotlightBackdrop (variant="poster-blur")
 *   6. Renders no disabled CTA (the dead "Add to list" button is gone)
 *   7. Renders exactly one .cta-hero "Watch" link
 *   8. Genre tags use color classes from cardTokens.anime_of_day.genreColors
 *   9. Uses only font-medium / font-semibold typography weights
 *  10. Tablet padding p-4 — never p-5
 *  11. Has no hardcoded English strings — every label flows through t()
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
    // Card components have a typed `data` prop; vue-tsc requires a concrete
    // shape rather than `Record<string, unknown>`. Cast at the boundary so
    // the helper stays generic across the three card specs.
    props: props as unknown as InstanceType<typeof AnimeOfDayCard>['$props'],
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
      { id: '1', name: 'Action', russian: 'Экшн' },
      { id: '8', name: 'Drama', russian: 'Драма' },
    ],
  },
}

describe('AnimeOfDayCard', () => {
  it('renders the localized title', () => {
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.text()).toContain('Frieren')
  })

  it('renders SpotlightBackdrop with poster-blur variant and the poster URL', () => {
    const wrapper = mountCard({ data: baseMockData })
    const backdrop = wrapper.findComponent({ name: 'SpotlightBackdrop' })
    expect(backdrop.exists()).toBe(true)
    expect(backdrop.props('variant')).toBe('poster-blur')
    expect(backdrop.props('posterUrl')).toBe(baseMockData.anime.poster_url)
  })

  it('renders the score badge as a meta-row pill (not an absolute overlay)', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    // New v1.1 styling moves the badge to a yellow-500/20 pill.
    expect(html).toContain('bg-yellow-500/20')
    expect(html).toContain('text-yellow-200')
    expect(wrapper.text()).toContain('9.1')
    // The pill must NOT be absolutely positioned over the poster.
    const pill = wrapper.find('.bg-yellow-500\\/20')
    expect(pill.exists()).toBe(true)
    expect(pill.classes()).not.toContain('absolute')
  })

  it('does not render score pill when score is undefined', () => {
    const data = {
      anime: { ...baseMockData.anime, score: undefined },
    }
    const wrapper = mountCard({ data })
    expect(wrapper.html()).not.toContain('bg-yellow-500/20')
    expect(wrapper.html()).not.toContain('text-yellow-200')
  })

  it('caps genres at 3', () => {
    const data = {
      anime: {
        ...baseMockData.anime,
        genres: [
          { id: '1', name: 'A', russian: 'А' },
          { id: '2', name: 'B', russian: 'Б' },
          { id: '4', name: 'C', russian: 'В' },
          { id: '8', name: 'D', russian: 'Г' },
          { id: '10', name: 'E', russian: 'Д' },
        ],
      },
    }
    const wrapper = mountCard({ data })
    // Three rendered chips correspond to the three mapped color classes.
    expect(wrapper.html()).toContain('bg-red-500/20')   // id=1 Action
    expect(wrapper.html()).toContain('bg-blue-500/20')  // id=2 Adventure
    expect(wrapper.html()).toContain('bg-yellow-500/20')// id=4 Comedy (also matches score, but score is also yellow — distinct entries)
    // The 4th + 5th genres (Drama/Fantasy) must NOT render.
    expect(wrapper.html()).not.toContain('bg-pink-500/20')
    expect(wrapper.html()).not.toContain('bg-purple-500/20')
  })

  it('does not render any disabled CTA (dead Add-to-list button is removed)', () => {
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.find('[aria-disabled="true"]').exists()).toBe(false)
    expect(wrapper.find('button[disabled]').exists()).toBe(false)
    // The i18n key for the dropped CTA must not appear in rendered text.
    expect(wrapper.text()).not.toContain('spotlight.animeOfDay.addCta')
  })

  it('renders exactly one .cta-hero CTA', () => {
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.findAll('.cta-hero')).toHaveLength(1)
  })

  it('applies genre color classes from cardTokens.anime_of_day.genreColors', () => {
    const wrapper = mountCard({
      data: {
        anime: {
          ...baseMockData.anime,
          // id=1 = Action → bg-red-500/20 text-red-200
          genres: [{ id: '1', name: 'Action', russian: 'Экшн' }],
        },
      },
    })
    expect(wrapper.html()).toContain('bg-red-500/20')
    expect(wrapper.html()).toContain('text-red-200')
  })

  it('falls back to neutral bg-white/10 for unmapped genre IDs', () => {
    const wrapper = mountCard({
      data: {
        anime: {
          ...baseMockData.anime,
          // id=9999 is intentionally outside the genreColors map.
          genres: [{ id: '9999', name: 'Unknown', russian: 'Неизвестно' }],
        },
      },
    })
    expect(wrapper.html()).toContain('bg-white/10')
    expect(wrapper.html()).toContain('text-gray-300')
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
    // Raw English copy should NOT leak — only the keys.
    expect(text).not.toMatch(/\bWatch\b/)
    expect(text).not.toMatch(/Add to list/)
  })
})
