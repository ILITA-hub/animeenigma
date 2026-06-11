/**
 * Workstream hero-spotlight — v4 B-1 lock (2026-06-11): deck-of-cards
 * discovery surface. Contract:
 *
 *   1. Single-root <article> (Transition out-in safety).
 *   2. Deck: poster + two ghost cards (.deck-g1/.deck-g2); the deal-in
 *      .deal class is applied on mount UNLESS prefers-reduced-motion.
 *   3. Density: dealtLabel tagline, status pill, year/episodes plain
 *      text, clamp-3 description.
 *   4. «Ещё разок» ghost button hits GET /home/spotlight/reroll?exclude=
 *      and swaps the shown anime from the response.
 *   5. Primary CTA routes to the anime detail page.
 *   6. Style discipline: font-medium/semibold only; p-4 ladder (no p-5).
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref, type Ref } from 'vue'
import { mount, RouterLinkStub, flushPromises } from '@vue/test-utils'

const mockReducedMotion: Ref<boolean> = ref(false)

vi.mock('@vueuse/core', () => ({
  useMediaQuery: () => mockReducedMotion,
}))

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

const apiGet = vi.fn()
vi.mock('@/api/client', () => ({
  apiClient: { get: (...args: unknown[]) => apiGet(...args) },
}))

// Poster preload resolves instantly in jsdom (no real Image loading);
// the call args still prove BOTH buckets (256 deck + 128 backdrop) warm.
const preload = vi.fn((..._args: unknown[]) => Promise.resolve())
vi.mock('@/utils/preload-image', () => ({
  preloadImage: (...args: unknown[]) => preload(...args),
  isImageWarm: () => false,
  markImageWarm: () => {},
}))

import RandomTailCard from './RandomTailCard.vue'

const baseAnime = {
  id: 'a-30',
  name: 'Neon Genesis Evangelion',
  name_ru: 'Евангелион',
  poster_url: '/poster.jpg',
  score: 8.37,
  year: 1995,
  episodes_count: 26,
  status: 'released',
  description: 'After the Second Impact…',
  genres: [
    { id: 1, name: 'Action', russian: 'Экшен' },
    { id: 2, name: 'Sci-Fi', russian: 'Фантастика' },
  ],
}

function mountCard(anime: Record<string, unknown> = baseAnime) {
  return mount(RandomTailCard, {
    props: { data: { anime } } as unknown as InstanceType<typeof RandomTailCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

beforeEach(() => {
  mockReducedMotion.value = false
  apiGet.mockReset()
  preload.mockClear()
})

describe('RandomTailCard (v4 B-1 deck)', () => {
  it('renders a single root <article> element', () => {
    const wrapper = mountCard()
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('renders the deck with two ghost cards behind the poster', () => {
    const wrapper = mountCard()
    const deck = wrapper.find('[data-testid="deck"]')
    expect(deck.exists()).toBe(true)
    expect(deck.find('.deck-g1').exists()).toBe(true)
    expect(deck.find('.deck-g2').exists()).toBe(true)
    expect(deck.find('img').exists()).toBe(true)
  })

  it('applies the deal-in class on mount (no reduced motion)', async () => {
    const wrapper = mountCard()
    await new Promise((r) => requestAnimationFrame(() => r(null)))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="deck"]').classes()).toContain('deal')
  })

  it('never applies the deal-in class under prefers-reduced-motion', async () => {
    mockReducedMotion.value = true
    const wrapper = mountCard()
    await new Promise((r) => requestAnimationFrame(() => r(null)))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="deck"]').classes()).not.toContain('deal')
  })

  it('renders the dealtLabel tagline + density meta (status, year, episodes, description)', () => {
    const wrapper = mountCard()
    const text = wrapper.text()
    expect(text).toContain('spotlight.randomTail.dealtLabel')
    expect(text).toContain('spotlight.randomTail.statusReleased')
    expect(text).toContain('1995')
    expect(text).toContain('spotlight.featured.episodesLabel')
    expect(wrapper.find('[data-testid="random-tail-desc"]').exists()).toBe(true)
    expect(
      wrapper.find('[data-testid="random-tail-desc"]').classes().join(' '),
    ).toContain('line-clamp-3')
  })

  it('reroll: shuffle loop plays while loading, poster preloads, then the anime swaps', async () => {
    vi.useFakeTimers()
    try {
      apiGet.mockResolvedValueOnce({
        data: {
          type: 'random_tail',
          data: { anime: { ...baseAnime, id: 'a-99', name: 'Mushishi', poster_url: '/m.jpg' } },
        },
      })
      const wrapper = mountCard()
      await wrapper.find('[data-testid="reroll-btn"]').trigger('click')
      // While the fetch + preload are in flight, the deck shuffles and the
      // OLD anime stays on screen (no half-loaded swap — 2026-06-11 fix).
      expect(wrapper.find('[data-testid="deck"]').classes()).toContain('shuffling')
      expect(wrapper.text()).toContain('Neon Genesis Evangelion')
      await flushPromises() // api + preload settle
      await vi.advanceTimersByTimeAsync(700) // MIN_SHUFFLE_MS floor elapses
      await flushPromises()
      expect(apiGet).toHaveBeenCalledWith('/home/spotlight/reroll?exclude=a-30')
      // Both image-proxy buckets were warmed before the swap.
      expect(preload).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).toContain('Mushishi')
      expect(wrapper.find('[data-testid="deck"]').classes()).not.toContain('shuffling')
    } finally {
      vi.useRealTimers()
    }
  })

  it('reroll under reduced motion: no shuffle class, swap still happens', async () => {
    mockReducedMotion.value = true
    apiGet.mockResolvedValueOnce({
      data: {
        type: 'random_tail',
        data: { anime: { ...baseAnime, id: 'a-77', name: 'Monster' } },
      },
    })
    const wrapper = mountCard()
    await wrapper.find('[data-testid="reroll-btn"]').trigger('click')
    expect(wrapper.find('[data-testid="deck"]').classes()).not.toContain('shuffling')
    await flushPromises()
    expect(wrapper.text()).toContain('Monster')
  })

  it('reroll failure keeps the current anime (console.warn path)', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    apiGet.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountCard()
    await wrapper.find('[data-testid="reroll-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Neon Genesis Evangelion')
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })

  it('primary CTA routes to the anime detail page', () => {
    const wrapper = mountCard()
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.some((l) => l.props('to') === '/anime/a-30')).toBe(true)
    expect(wrapper.text()).toContain('spotlight.randomTail.discoverCta')
  })

  it('style discipline: no font-bold, no p-5', () => {
    const html = mountCard().html()
    expect(html).not.toMatch(/font-bold/)
    expect(html).not.toMatch(/\bp-5\b/)
  })
})
