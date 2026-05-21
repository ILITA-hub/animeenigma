/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-04 / Task 2.
 *
 * Vitest spec for HeroSpotlightBlock.vue. Verifies the carousel state
 * machine end-to-end via mounted DOM + fake timers:
 *
 *   1. Skeleton renders during loading
 *   2. Section renders with role=region after cards populate
 *   3. Section does NOT render when cards.length === 0 after load
 *   4. Section does NOT render when feature flag set to 'false'
 *   5. Initial slide is randomized after cards populate (statistical test)
 *   6. Auto-cycle advances currentIndex every 7000ms
 *   7. Wraparound — clicking next at last index goes back to 0
 *   8. Reduced-motion disables auto-cycle (no advance after 8000ms)
 *   9. Single-card response does NOT auto-cycle
 *  10. aria-live=polite present ONLY on slide container, not on section
 *  11. Slide aria-label uses spotlight.slideLabelWithTitle key (carries n/total)
 *  12. Unknown card type renders silently (no console.error, no crash)
 *
 * Mocking strategy:
 *  - @/composables/useSpotlight is fully replaced with a reactive shim so
 *    the test controls loading + cards + error.
 *  - @vueuse/core is partial-mocked: useMediaQuery returns a controllable
 *    ref for prefers-reduced-motion (other queries fall back to ref(false)).
 *    useIntervalFn falls through to the real implementation so vi.useFakeTimers
 *    can drive the 7000ms advance.
 *  - vue-i18n's t() echoes key + JSON-encoded params so aria-label assertions
 *    can be string-matched without loading the locale JSON.
 *  - getLocalizedTitle stubbed to first-non-empty passthrough.
 *  - Child card components are auto-mounted; router-link is stubbed.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref, type Ref } from 'vue'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import type { SpotlightCard } from '@/types/spotlight'

// ── Composable mock — reactive shim the tests can mutate ────────────────────
const mockState: {
  cards: Ref<SpotlightCard[]>
  loading: Ref<boolean>
  error: Ref<Error | null>
  refresh: ReturnType<typeof vi.fn>
} = {
  cards: ref<SpotlightCard[]>([]),
  loading: ref(true),
  error: ref(null),
  refresh: vi.fn(),
}

vi.mock('@/composables/useSpotlight', () => ({
  useSpotlight: () => mockState,
}))

// ── @vueuse/core mock — controllable reducedMotion, real useIntervalFn ─────
const mockReducedMotion = ref(false)
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

// ── vue-i18n mock — echoes key + params so we can assert against patterns ──
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: ref('en'),
  }),
}))

// ── title helper stub ───────────────────────────────────────────────────────
vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

// Imported AFTER vi.mock so the SFC's deps resolve to the stubs.
import HeroSpotlightBlock from './HeroSpotlightBlock.vue'

// ── Helpers ─────────────────────────────────────────────────────────────────
function mockCards(n: number): SpotlightCard[] {
  return Array.from({ length: n }, (_, i) => ({
    type: 'anime_of_day' as const,
    data: {
      anime: {
        id: `anime-${i}`,
        name: `Anime ${i}`,
        name_ru: `Аниме ${i}`,
      },
    },
  }))
}

function mountBlock() {
  return mount(HeroSpotlightBlock, {
    global: {
      stubs: { 'router-link': RouterLinkStub },
    },
  })
}

/**
 * Parses the current slide's aria-label which is encoded as
 * `spotlight.slideLabelWithTitle::{"n":2,"total":4,"title":"..."}` thanks to
 * the t() mock. Returns the n-value as 0-indexed currentIndex.
 *
 * Reads the attribute via vue-test-utils' attributes() helper (which returns
 * the unescaped string), NOT via wrapper.html() which encodes `"` as `&quot;`.
 */
function readActiveIndex(wrapper: ReturnType<typeof mount>): number {
  const slide = wrapper.find('[role="group"]')
  if (!slide.exists()) return -1
  const label = slide.attributes('aria-label') ?? ''
  const m = label.match(/"n":(\d+)/)
  if (!m) return -1
  return parseInt(m[1], 10) - 1
}

beforeEach(() => {
  mockState.cards.value = []
  mockState.loading.value = true
  mockState.error.value = null
  mockReducedMotion.value = false
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllEnvs()
})

describe('HeroSpotlightBlock', () => {
  it('renders skeleton when loading=true', () => {
    mockState.loading.value = true
    mockState.cards.value = []
    const wrapper = mountBlock()

    // Skeleton wrapper carries aria-hidden="true" and the .skeleton-shimmer
    // class. The full <section role="region"> must NOT be present.
    expect(wrapper.find('[aria-hidden="true"]').exists()).toBe(true)
    expect(wrapper.find('.skeleton-shimmer').exists()).toBe(true)
    expect(wrapper.find('section[role="region"]').exists()).toBe(false)
  })

  it('renders section with role=region when cards populate', async () => {
    // Cards transition 0 → 4 AFTER mount so the SFC's
    // watch(() => cards.value.length) fires and seeds currentIndex
    // (Pitfall 4 mitigation behavior).
    mockState.loading.value = false
    mockState.cards.value = []
    const wrapper = mountBlock()
    mockState.cards.value = mockCards(4)
    await flushPromises()

    expect(wrapper.find('section[role="region"]').exists()).toBe(true)
    expect(wrapper.find('[aria-roledescription="carousel"]').exists()).toBe(true)
    expect(wrapper.find('[aria-roledescription="slide"]').exists()).toBe(true)
  })

  it('does NOT render section when cards.length===0 after load', async () => {
    mockState.loading.value = false
    mockState.cards.value = []
    const wrapper = mountBlock()
    await flushPromises()

    expect(wrapper.find('section[role="region"]').exists()).toBe(false)
    // Skeleton is also gone — loading is false.
    expect(wrapper.find('.skeleton-shimmer').exists()).toBe(false)
    // Component is fully self-hidden — no rendered DOM elements at all
    // (only comment nodes from the v-if/v-else-if branches remain).
    expect(wrapper.find('div').exists()).toBe(false)
    expect(wrapper.find('section').exists()).toBe(false)
  })

  it('does NOT render section when feature flag is set to "false"', async () => {
    // Pitfall 6 in RESEARCH.md — import.meta.env mocking in Vitest is
    // ordering-sensitive. We use vi.stubEnv before mount to override the
    // VITE_HERO_SPOTLIGHT_ENABLED key. Vite exposes import.meta.env.* and
    // Vitest's stubEnv populates that object at runtime.
    vi.stubEnv('VITE_HERO_SPOTLIGHT_ENABLED', 'false')
    // Re-import to pick up the stubbed env. The component reads
    // import.meta.env at the top level of <script setup>, which is
    // re-evaluated each `mount()` because Vue creates a fresh setup scope.
    mockState.loading.value = false
    mockState.cards.value = mockCards(4)
    // vi.resetModules so the static `const enabled = ...` line at the top
    // of the SFC's <script setup> re-evaluates against the stubbed env.
    vi.resetModules()
    const fresh = await import('./HeroSpotlightBlock.vue')
    const wrapper = mount(fresh.default, {
      global: { stubs: { 'router-link': RouterLinkStub } },
    })
    await flushPromises()

    expect(wrapper.find('section[role="region"]').exists()).toBe(false)
    expect(wrapper.find('.skeleton-shimmer').exists()).toBe(false)
  })

  it('randomizes currentIndex after cards populate (statistical)', async () => {
    // With 4 cards × 30 mounts the chance of getting all-same-index is
    // 4 × (1/4)^30 ≈ 3.5e-18 — effectively zero. We assert ≥2 distinct
    // values which is a robust randomization signal.
    //
    // CRITICAL: the SFC's `watch(() => cards.value.length, ...)` uses
    // `{immediate: false}`. It only fires when length transitions, so each
    // trial must reset `cards.value = []` BEFORE mount, then populate AFTER
    // mount — otherwise the random init never runs and we always see 0.
    const observed = new Set<number>()
    for (let trial = 0; trial < 30; trial++) {
      mockState.loading.value = false
      mockState.cards.value = []
      const wrapper = mountBlock()
      mockState.cards.value = mockCards(4)
      await flushPromises()
      const idx = readActiveIndex(wrapper)
      observed.add(idx)
      wrapper.unmount()
    }
    expect(observed.size).toBeGreaterThanOrEqual(2)
  })

  it('advances currentIndex by 1 every 7000ms', async () => {
    vi.useFakeTimers()
    mockState.loading.value = false
    // Reset to [] before mount so the watch fires when we populate post-mount
    // (this is what kicks off startCycle inside the SFC).
    mockState.cards.value = []
    const wrapper = mountBlock()
    mockState.cards.value = mockCards(4)
    await flushPromises()

    const initial = readActiveIndex(wrapper)
    expect(initial).toBeGreaterThanOrEqual(0)

    vi.advanceTimersByTime(7000)
    await flushPromises()

    const next = readActiveIndex(wrapper)
    // With wraparound, next == (initial + 1) % 4
    expect(next).toBe((initial + 1) % 4)
  })

  it('wraps around from last card to first via the next chevron', async () => {
    vi.useFakeTimers()
    mockState.loading.value = false
    mockState.cards.value = mockCards(3)
    const wrapper = mountBlock()
    await flushPromises()

    // We don't control the random initial index — manually click "next" until
    // we hit the last index (2), then click once more to verify wraparound to 0.
    const nextBtn = wrapper.find('[aria-label="spotlight.nextSlide"]')
    expect(nextBtn.exists()).toBe(true)

    // Drive forward up to 4 times — guarantees we visit index 2 then 0.
    let lastIdx = readActiveIndex(wrapper)
    let sawZeroAfterTwo = false
    for (let i = 0; i < 6 && !sawZeroAfterTwo; i++) {
      const prev = lastIdx
      await nextBtn.trigger('click')
      await flushPromises()
      lastIdx = readActiveIndex(wrapper)
      if (prev === 2 && lastIdx === 0) sawZeroAfterTwo = true
    }
    expect(sawZeroAfterTwo).toBe(true)
  })

  it('does NOT advance when reducedMotion is true', async () => {
    vi.useFakeTimers()
    mockReducedMotion.value = true
    mockState.loading.value = false
    mockState.cards.value = mockCards(4)
    const wrapper = mountBlock()
    await flushPromises()

    const initial = readActiveIndex(wrapper)
    vi.advanceTimersByTime(8000)
    await flushPromises()

    expect(readActiveIndex(wrapper)).toBe(initial)
  })

  it('does NOT advance when cards.length===1', async () => {
    vi.useFakeTimers()
    mockState.loading.value = false
    mockState.cards.value = mockCards(1)
    const wrapper = mountBlock()
    await flushPromises()

    const initial = readActiveIndex(wrapper)
    vi.advanceTimersByTime(8000)
    await flushPromises()

    expect(readActiveIndex(wrapper)).toBe(initial)
    // Only 1 card — only 1 dot button rendered.
    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(1)
  })

  it('has aria-live=polite on slide container only, NOT on section wrapper', async () => {
    mockState.loading.value = false
    mockState.cards.value = mockCards(2)
    const wrapper = mountBlock()
    await flushPromises()

    // Section itself must NOT carry aria-live.
    const section = wrapper.find('section[role="region"]')
    expect(section.exists()).toBe(true)
    expect(section.attributes('aria-live')).toBeUndefined()

    // Slide container must carry aria-live=polite.
    const slide = wrapper.find('[role="group"][aria-live="polite"]')
    expect(slide.exists()).toBe(true)
    expect(slide.attributes('aria-roledescription')).toBe('slide')
    expect(slide.attributes('aria-atomic')).toBe('true')
  })

  it('renders slide aria-label of form spotlight.slideLabelWithTitle with n/total params', async () => {
    mockState.loading.value = false
    mockState.cards.value = mockCards(3)
    const wrapper = mountBlock()
    await flushPromises()

    const slide = wrapper.find('[role="group"]')
    const label = slide.attributes('aria-label') ?? ''
    expect(label).toContain('spotlight.slideLabelWithTitle')
    expect(label).toContain('"total":3')
    // n is 1..3 (1-indexed). Match any of those values.
    expect(label).toMatch(/"n":[1-3]/)
  })

  it('renders without console.error when an unknown card type is encountered', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    mockState.loading.value = false
    // Cast through unknown — we want to simulate a forward-compat scenario
    // where the backend ships a variant the frontend doesn't yet know.
    mockState.cards.value = [
      { type: 'unknown', data: {} } as unknown as SpotlightCard,
    ]
    const wrapper = mountBlock()
    await flushPromises()

    // Section still renders (cards.length > 0); the <component :is> resolves
    // to undefined which Vue renders as nothing — no thrown error expected.
    expect(wrapper.find('section[role="region"]').exists()).toBe(true)
    expect(errSpy).not.toHaveBeenCalled()
    errSpy.mockRestore()
  })
})
