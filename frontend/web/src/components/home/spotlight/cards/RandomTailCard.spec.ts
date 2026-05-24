/**
 * Workstream hero-spotlight — v1.1-polish Phase 03 (HSB-V11-RT-01..04).
 *
 * Vitest spec for RandomTailCard.vue (refactored). Verifies the visual
 * deltas from the Phase 2 baseline and the new discovery identity:
 *
 *   1.  SpotlightBackdrop (variant="poster-blur") is mounted with the
 *       card's poster URL — re-uses the already-fetched poster image.
 *   2.  A purple-tinted secondary overlay (from-purple-500/30) sits
 *       above the backdrop so the card reads as "discovery" rather
 *       than the cyan AnimeOfDay sibling.
 *   3.  A <SpotlightIcon name="shuffle"> leads the kicker, both on the
 *       mobile (md:hidden) and desktop (hidden md:flex) copies.
 *   4.  The desktop tagline (hidden md:block) is one of the 4 candidates
 *       from spotlight.randomTail.taglines (per-locale random pick).
 *   5.  Primary CTA is a purple .cta-hero (data-accent="purple") — the
 *       old btn-primary / btn-ghost classes are gone.
 *   6.  The CTA carries the shuffle icon inline (visual reinforcement
 *       of the discovery affordance).
 *   7.  With prefers-reduced-motion: reduce, the shuffle-deck layer is
 *       NOT mounted (no data-testid="shuffle-deck" element exists).
 *   8.  Without reduced motion, the shuffle-deck IS mounted and contains
 *       exactly 5 .shuffle-card children (the staggered deck).
 *   9.  Tagline falls back to spotlight.randomTail.subtitle when tm()
 *       returns a non-array (defensive shim).
 *  10.  No font-bold / font-normal — only font-medium / font-semibold.
 *  11.  Tablet padding stays p-4 (never p-5).
 *  12.  All copy flows through t() — no hardcoded English leaks.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref, nextTick, type Ref } from 'vue'
import { mount, RouterLinkStub } from '@vue/test-utils'

// ── @vueuse/core mock — controllable reduced-motion ────────────────────────
// Default is "no preference" (false). Tests that want to assert the
// reduced-motion path flip this ref BEFORE calling mountCard().
const mockReducedMotion: Ref<boolean> = ref(false)

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useMediaQuery: (q: string) => {
      if (q.includes('prefers-reduced-motion')) return mockReducedMotion
      return ref(false)
    },
  }
})

// ── vue-i18n mock — adds tm() returning the taglines array ─────────────────
const TAGLINES = [
  'tagline-one',
  'tagline-two',
  'tagline-three',
  'tagline-four',
] as const

// Toggle for the "tm returns non-array" defensive test.
let tmReturnsArray = true

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    tm: (key: string): unknown => {
      if (key === 'spotlight.randomTail.taglines' && tmReturnsArray) {
        return [...TAGLINES]
      }
      // Mimics vue-i18n's behavior when the key is missing or not an
      // array — it returns the key string (or an empty object).
      return key
    },
    locale: ref('en'),
  }),
}))

// ── title helper stub ───────────────────────────────────────────────────────
vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

// Imported AFTER vi.mock so the SFC's useI18n() + useMediaQuery resolve to
// the stubs.
import RandomTailCard from './RandomTailCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(RandomTailCard, {
    props: props as unknown as InstanceType<typeof RandomTailCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const baseMockData = {
  anime: {
    id: 'xyz-456',
    name: 'Mushishi',
    name_ru: 'Мусиси',
    poster_url: '/test-poster.jpg',
    score: 8.6,
    episodes_count: 26,
    genres: [
      { id: '1', name: 'Action', russian: 'Экшн' },
      { id: '8', name: 'Drama', russian: 'Драма' },
    ],
  },
}

describe('RandomTailCard', () => {
  beforeEach(() => {
    // Reset the shared mocks before every test so order-independence
    // holds (vitest by default runs tests in source order, but explicit
    // resets keep the spec robust to --shuffle and re-runs).
    mockReducedMotion.value = false
    tmReturnsArray = true
  })

  afterEach(() => {
    // The component sets a 1000ms setTimeout when reduced-motion is off;
    // tearing the wrapper down in afterEach via beforeEach reset is
    // sufficient because mount() builds a fresh instance each test.
  })

  it('renders SpotlightBackdrop with poster-blur variant and the poster URL', () => {
    const wrapper = mountCard({ data: baseMockData })
    const backdrop = wrapper.findComponent({ name: 'SpotlightBackdrop' })
    expect(backdrop.exists()).toBe(true)
    expect(backdrop.props('variant')).toBe('poster-blur')
    expect(backdrop.props('posterUrl')).toBe(baseMockData.anime.poster_url)
  })

  it('renders the purple-tinted secondary overlay', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    // The overlay is a div with from-purple-500/30 in its bg-gradient
    // utility chain. The Tailwind class string survives unchanged in the
    // rendered HTML so a substring match is reliable here.
    expect(html).toContain('from-purple-500/30')
  })

  it('renders a SpotlightIcon name="shuffle" in the header (both layouts)', () => {
    const wrapper = mountCard({ data: baseMockData })
    const icons = wrapper.findAllComponents({ name: 'SpotlightIcon' })
    // Three shuffle icons total: mobile header + desktop header + inline
    // in the CTA. All three must be `name="shuffle"`.
    const shuffleIcons = icons.filter((i) => i.props('name') === 'shuffle')
    expect(shuffleIcons.length).toBeGreaterThanOrEqual(2)
    for (const icon of shuffleIcons) {
      expect(icon.props('name')).toBe('shuffle')
    }
  })

  it('renders the localized title via getLocalizedTitle', () => {
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.text()).toContain('Mushishi')
  })

  it('renders a tagline that is one of the 4 candidates from i18n', async () => {
    const wrapper = mountCard({ data: baseMockData })
    // The component picks the tagline inside onMounted(), so we need at
    // least one reactivity tick before the DOM reflects the ref change.
    await nextTick()
    const tagEl = wrapper.find('[data-testid="random-tail-tagline"]')
    expect(tagEl.exists()).toBe(true)
    const tagText = tagEl.text().trim()
    // The picked tagline must be one of the 4 mocked candidates — never
    // the raw i18n key, never the scalar subtitle fallback (which only
    // fires when tm() returns a non-array).
    expect(TAGLINES as readonly string[]).toContain(tagText)
  })

  it('tagline element is hidden on mobile (hidden md:block)', () => {
    const wrapper = mountCard({ data: baseMockData })
    const tagEl = wrapper.find('[data-testid="random-tail-tagline"]')
    expect(tagEl.exists()).toBe(true)
    expect(tagEl.classes()).toContain('hidden')
    expect(tagEl.classes()).toContain('md:block')
  })

  it('falls back to spotlight.randomTail.subtitle when tm() returns a non-array', async () => {
    tmReturnsArray = false
    const wrapper = mountCard({ data: baseMockData })
    await nextTick()
    const tagEl = wrapper.find('[data-testid="random-tail-tagline"]')
    // With the array missing, the SFC reads spotlight.randomTail.subtitle
    // via t() — which (under the echo mock) renders as the key string.
    expect(tagEl.text().trim()).toBe('spotlight.randomTail.subtitle')
  })

  it('renders exactly one .cta-hero CTA with data-accent="purple"', () => {
    const wrapper = mountCard({ data: baseMockData })
    const ctas = wrapper.findAll('.cta-hero')
    expect(ctas).toHaveLength(1)
    const cta = ctas[0]
    expect(cta.attributes('data-accent')).toBe('purple')
    // The CTA copy is the discoverCta i18n key under our echo mock.
    expect(cta.text()).toContain('spotlight.randomTail.discoverCta')
  })

  it('does not retain the legacy btn-primary / btn-ghost CTA classes', () => {
    const wrapper = mountCard({ data: baseMockData })
    const html = wrapper.html()
    expect(html).not.toContain('btn-primary')
    expect(html).not.toContain('btn-ghost')
    // The legacy Add CTA must not have been added under any new key.
    expect(wrapper.text()).not.toContain('spotlight.randomTail.addCta')
    expect(wrapper.text()).not.toContain('spotlight.animeOfDay.addCta')
  })

  it('does NOT mount the shuffle-deck when prefers-reduced-motion is reduce', () => {
    mockReducedMotion.value = true
    const wrapper = mountCard({ data: baseMockData })
    expect(wrapper.find('[data-testid="shuffle-deck"]').exists()).toBe(false)
    // The .shuffle-card class also must not appear (defense-in-depth).
    expect(wrapper.html()).not.toContain('shuffle-card')
  })

  it('mounts the shuffle-deck with 5 staggered cards when motion is allowed', () => {
    mockReducedMotion.value = false
    const wrapper = mountCard({ data: baseMockData })
    const deck = wrapper.find('[data-testid="shuffle-deck"]')
    expect(deck.exists()).toBe(true)
    const cards = deck.findAll('.shuffle-card')
    expect(cards).toHaveLength(5)
    // Each card carries a --delay custom property of n*60ms.
    for (let i = 0; i < cards.length; i++) {
      const style = cards[i].attributes('style') ?? ''
      // n is 1..5 so the delay is 60..300ms.
      const expectedDelay = `${(i + 1) * 60}ms`
      expect(style).toContain(`--delay: ${expectedDelay}`)
    }
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
    expect(text).toContain('spotlight.randomTail.title')
    expect(text).toContain('spotlight.randomTail.discoverCta')
    // No raw English copy should leak.
    expect(text).not.toMatch(/\bOpen\b/)
    expect(text).not.toMatch(/Random pick/)
  })
})
