/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-01 / Task 2.
 *
 * Vitest spec for useSpotlight() composable. Stubs `@/api/client` apiClient
 * and verifies the fetch / unwrap / silent-self-hide-on-error contract.
 *
 * Five required cases (per Plan 02-01 acceptance criteria):
 *   1. fetch on mount + populates cards on raw {cards, generated_at}
 *   2. unwraps wrapped envelope {success, data:{cards, generated_at}}
 *   3. 5xx error path → empty cards + one console.warn + error ref set
 *   4. defensive null-cards path → empty cards (Array.isArray guard)
 *   5. refresh() re-runs the fetch
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h, nextTick, type Ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import type { SpotlightCard } from '@/types/spotlight'
import type { useSpotlight as UseSpotlightFn } from './useSpotlight'

// vi.mock factory runs BEFORE imports so the composable's `apiClient` resolves
// to the spy. Returning a Proxy-with-vi-fn-getters is overkill; a flat object
// with a single .get vi.fn() matches the call site in useSpotlight.ts.
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn() },
}))

// The composable keeps a MODULE-LEVEL cards cache (2026-07-04 route-revisit
// dedupe), so every test must get a fresh module instance — resetModules +
// dynamic import in beforeEach; static imports would share one cache across
// tests and make case order matter.
let useSpotlight: typeof UseSpotlightFn
let apiGetSpy: ReturnType<typeof vi.fn>

// Public shape the harness exposes — the composable's refs are the same
// references we read inside the tests, so we treat them as the underlying
// reactive types (Vue auto-unwraps in templates, but our test-utils vm
// access also auto-unwraps shallow refs).
interface HarnessVm {
  cards: SpotlightCard[]
  loading: boolean
  error: Error | null
  refresh: () => Promise<void>
}

/**
 * Tiny harness component that exposes the composable's refs on its instance
 * so the test can assert against them after mount. Using a render function
 * keeps this dependency-free of any SFC compiler quirks.
 *
 * We deliberately type setup() return as Refs so the harness component's
 * exposed props auto-unwrap when accessed via `wrapper.vm.cards` in tests.
 */
function mountHarness() {
  const Harness = defineComponent({
    setup(): {
      cards: Ref<SpotlightCard[]>
      loading: Ref<boolean>
      error: Ref<Error | null>
      refresh: () => Promise<void>
    } {
      const spot = useSpotlight()
      // Expose via setup return so vm.<name> works in the test
      return spot
    },
    render() {
      return h('div')
    },
  })
  return mount(Harness)
}

describe('useSpotlight', () => {
  let warnSpy: ReturnType<typeof vi.spyOn>

  beforeEach(async () => {
    vi.resetModules()
    const client = await import('@/api/client')
    apiGetSpy = client.apiClient.get as ReturnType<typeof vi.fn>
    apiGetSpy.mockReset()
    ;({ useSpotlight } = await import('./useSpotlight'))
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    warnSpy.mockRestore()
  })

  it('fetches on mount and populates cards when API returns {cards, generated_at}', async () => {
    apiGetSpy.mockResolvedValueOnce({
      data: {
        cards: [
          { type: 'featured', data: { anime: { id: 'a-1' } } },
        ],
        generated_at: '2026-05-21T03:37:10Z',
      },
    })

    const wrapper = mountHarness()
    await flushPromises()
    await nextTick()

    const vm = wrapper.vm as unknown as HarnessVm
    expect(apiGetSpy).toHaveBeenCalledTimes(1)
    expect(apiGetSpy).toHaveBeenCalledWith('/home/spotlight')
    expect(vm.cards.length).toBe(1)
    expect(vm.cards[0].type).toBe('featured')
    expect(vm.loading).toBe(false)
    expect(vm.error).toBeNull()
  })

  it('unwraps wrapped envelope {success, data:{cards, generated_at}}', async () => {
    apiGetSpy.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          cards: [
            { type: 'latest_news', data: { entries: [] } },
            { type: 'platform_stats', data: { metrics: [] } },
          ],
          generated_at: '2026-05-21T03:37:10Z',
        },
      },
    })

    const wrapper = mountHarness()
    await flushPromises()

    const vm = wrapper.vm as unknown as HarnessVm
    expect(vm.cards.length).toBe(2)
    expect(vm.cards[0].type).toBe('latest_news')
    expect(vm.cards[1].type).toBe('platform_stats')
    expect(vm.loading).toBe(false)
    expect(vm.error).toBeNull()
  })

  it('returns empty cards on 5xx and emits one console.warn', async () => {
    apiGetSpy.mockRejectedValueOnce(new Error('Network 503'))

    const wrapper = mountHarness()
    await flushPromises()

    const vm = wrapper.vm as unknown as HarnessVm
    expect(vm.cards.length).toBe(0)
    expect(vm.error).toBeInstanceOf(Error)
    expect(vm.loading).toBe(false)

    // Exactly one warn; message prefix matches the contract for grep-based
    // observability (acceptance criterion).
    expect(warnSpy).toHaveBeenCalledTimes(1)
    const firstArg = warnSpy.mock.calls[0][0] as string
    expect(firstArg).toMatch(/^\[spotlight\] fetch failed/)
  })

  it('returns empty cards when response body cards field is null', async () => {
    apiGetSpy.mockResolvedValueOnce({
      data: {
        cards: null, // defensive: backend hiccup or partial-success shape
        generated_at: '2026-05-21T03:37:10Z',
      },
    })

    const wrapper = mountHarness()
    await flushPromises()

    const vm = wrapper.vm as unknown as HarnessVm
    expect(vm.cards.length).toBe(0)
    expect(vm.loading).toBe(false)
    // No throw → no warn, no error ref. The block self-hides on cards.length===0.
    expect(vm.error).toBeNull()
    expect(warnSpy).not.toHaveBeenCalled()
  })

  it('exposes refresh() that re-runs the fetch', async () => {
    // First fetch on mount
    apiGetSpy.mockResolvedValueOnce({
      data: {
        cards: [{ type: 'featured', data: { anime: { id: 'first' } } }],
        generated_at: '2026-05-21T03:37:10Z',
      },
    })
    const wrapper = mountHarness()
    await flushPromises()

    const vm = wrapper.vm as unknown as HarnessVm
    expect(vm.cards.length).toBe(1)
    expect((vm.cards[0] as { data: { anime: { id: string } } }).data.anime.id).toBe('first')

    // Second fetch via refresh()
    apiGetSpy.mockResolvedValueOnce({
      data: {
        cards: [
          { type: 'featured', data: { anime: { id: 'second-a' } } },
          { type: 'random_tail', data: { anime: { id: 'second-b' } } },
        ],
        generated_at: '2026-05-21T03:40:00Z',
      },
    })
    await vm.refresh()
    await flushPromises()

    expect(apiGetSpy).toHaveBeenCalledTimes(2)
    expect(vm.cards.length).toBe(2)
    expect((vm.cards[0] as { data: { anime: { id: string } } }).data.anime.id).toBe('second-a')
  })

  it('serves a second mount from the module cache without refetching', async () => {
    apiGetSpy.mockResolvedValueOnce({
      data: {
        cards: [{ type: 'featured', data: { anime: { id: 'cached' } } }],
        generated_at: '2026-07-04T10:00:00Z',
      },
    })

    mountHarness()
    await flushPromises()
    expect(apiGetSpy).toHaveBeenCalledTimes(1)

    // SPA route revisit: a fresh mount inside the TTL reuses the shared
    // cards and issues NO second request (2026-07-04 dedupe contract).
    const second = mountHarness()
    await flushPromises()

    const vm = second.vm as unknown as HarnessVm
    expect(apiGetSpy).toHaveBeenCalledTimes(1)
    expect(vm.cards.length).toBe(1)
    expect(vm.loading).toBe(false)
  })
})
