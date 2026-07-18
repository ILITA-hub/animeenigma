/**
 * Streaming smoke test for FanficsView.vue's core wiring: GenerateForm emits
 * `generate` -> fanficApi.generate() SSE handlers accumulate into the
 * reactive `content`/`generating` state -> onDone flips `generating` back
 * off and (when the library tab happens to already be mounted, see the
 * `libraryGridRef` comment in FanficsView.vue) tells LibraryGrid to refresh.
 *
 * GenerateForm is stubbed (its own behavior is covered by
 * src/components/fanfic/__tests__/GenerateForm.spec.ts) so this test stays
 * focused on the streaming plumbing. FanficReader/LibraryGrid/Tabs/Modal are
 * left real — they're simple enough not to need stubbing and this doubles as
 * a check that FanficsView's template wiring to them doesn't throw.
 *
 * `activeTab`/`generating`/`content`/`genTitle`/`genError`/`libraryGridRef`/
 * `onGenerate` are exposed via defineExpose in FanficsView.vue specifically
 * for this test: Tabs only ever mounts ONE of #generate/#library at a time
 * (a single `<slot :name="modelValue" />`), so `libraryGridRef` is only
 * populated in the real app when the user switches to the library tab
 * *while a generation is still streaming* — a DOM-only test can't reach that
 * window without reproducing real SSE timing, so we drive it directly.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import FanficsView from '../FanficsView.vue'
import type { GenerateInput, StreamHandlers } from '@/types/fanfic'

// vi.mock factories are hoisted above imports — anything they close over
// must itself be created via vi.hoisted() (see GenerateForm.spec.ts).
const { generateMock, releaseGenerate, getDailyMock } = vi.hoisted(() => {
  let release: (() => void) | null = null
  const generateMock = vi.fn(
    async (_input: unknown, handlers: StreamHandlers) => {
      // Mirrors the real fanficApi.generate: dispatches meta + deltas as
      // they'd arrive off the SSE stream, then suspends (like the pending
      // network read) until the test releases it, so `generating` can be
      // observed true BEFORE onDone flips it back off.
      handlers.onMeta?.('fic-1', 'gpt-test')
      handlers.onDelta?.('# T\n\n')
      handlers.onDelta?.('body')
      await new Promise<void>((resolve) => {
        release = resolve
      })
      handlers.onDone?.('fic-1', 'T', 10)
    },
  )
  const getDailyMock = vi.fn()
  return { generateMock, releaseGenerate: () => release?.(), getDailyMock }
})

vi.mock('@/api/fanfic', () => ({
  fanficApi: {
    generate: generateMock,
    list: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    get: vi.fn(),
    remove: vi.fn(),
    tags: vi.fn().mockResolvedValue([]),
    getDaily: getDailyMock,
  },
}))

// route.query is mutated per-test via routeQueryRef.value before mounting —
// FanficsView only reads route.query.daily once, in onMounted.
const { routeQueryRef, routerReplaceMock } = vi.hoisted(() => ({
  routeQueryRef: { value: {} as Record<string, string> },
  routerReplaceMock: vi.fn(),
}))
vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ query: routeQueryRef.value }),
    useRouter: () => ({ replace: routerReplaceMock }),
  }
})

// Fanfic feature visibility (utils/fanficGate -> policy feed + pinia). Mocked
// to a mutable holder so tests can flip between a full authoring viewer
// (true, the default) and a daily-reader-only viewer (false) without booting
// pinia. Wrapped in a real ComputedRef inside the factory so the template's
// `v-if="fanficVisible"` unwraps it like the real composable's return.
const { fanficVisibleHolder } = vi.hoisted(() => ({
  fanficVisibleHolder: { value: true },
}))
vi.mock('@/utils/fanficGate', async () => {
  const { computed } = await import('vue')
  return { useFanficVisible: () => computed(() => fanficVisibleHolder.value) }
})

const pushToastMock = vi.fn()
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: pushToastMock, toasts: { value: [] }, dismiss: vi.fn() }),
}))

// GenerateForm has its own heavy deps (anime search, characters, tags) fully
// covered by GenerateForm.spec.ts — stub it here to a bare emitter so this
// test only exercises FanficsView's own streaming wiring.
vi.mock('@/components/fanfic/GenerateForm.vue', () => ({
  default: {
    name: 'GenerateForm',
    props: ['disabled'],
    emits: ['generate'],
    template: '<div data-testid="generate-form-stub" />',
  },
}))

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en, ru, ja } })

const fakeInput: GenerateInput = {
  anime: { id: 'a1', title: 'Test Anime' },
  characters: [],
  tags: [],
  length: 'oneshot',
  pov: 'third',
  rating: 'teen',
  language: 'ru',
  prompt: 'A cozy story',
}

interface FanficsViewVm {
  activeTab: 'generate' | 'library'
  generating: boolean
  content: string
  genTitle: string
  genError: string
  libraryGridRef: { refresh: () => void } | null
  onGenerate: (input: GenerateInput) => Promise<void>
  readerOpen: boolean
  readerFanfic: { id: string; title: string; content: string } | null
  readerIsDaily: boolean
}

function mountView() {
  return mount(FanficsView, { global: { plugins: [i18n] } })
}

describe('FanficsView streaming', () => {
  it('accumulates deltas, toggles `generating` true->false, and refreshes the library on done', async () => {
    const wrapper = mountView()
    const vm = wrapper.vm as unknown as FanficsViewVm

    // Simulate the library tab already being mounted (see file-header note).
    const refreshSpy = vi.fn()
    vm.libraryGridRef = { refresh: refreshSpy }

    const form = wrapper.findComponent({ name: 'GenerateForm' })
    expect(form.exists()).toBe(true)
    form.vm.$emit('generate', fakeInput)

    expect(generateMock).toHaveBeenCalledTimes(1)
    expect(generateMock.mock.calls[0][0]).toEqual(fakeInput)

    // Mid-stream: both deltas landed, generation is still in flight (the mock
    // hasn't been released yet), nothing done-related has fired.
    expect(vm.generating).toBe(true)
    expect(vm.content).toBe('# T\n\nbody')
    expect(vm.genTitle).toBe('')
    expect(refreshSpy).not.toHaveBeenCalled()

    releaseGenerate()
    await flushPromises()

    // Post-done: streaming flag flipped back off, title landed, library told
    // to refresh exactly once.
    expect(vm.generating).toBe(false)
    expect(vm.genTitle).toBe('T')
    expect(vm.genError).toBe('')
    expect(refreshSpy).toHaveBeenCalledTimes(1)
  })

  it('surfaces onError and stops the streaming flag without refreshing the library', async () => {
    generateMock.mockImplementationOnce(async (_input, handlers) => {
      handlers.onDelta?.('partial')
      handlers.onError?.('boom')
    })

    const wrapper = mountView()
    const vm = wrapper.vm as unknown as FanficsViewVm
    const refreshSpy = vi.fn()
    vm.libraryGridRef = { refresh: refreshSpy }

    const form = wrapper.findComponent({ name: 'GenerateForm' })
    form.vm.$emit('generate', fakeInput)
    await flushPromises()

    expect(vm.generating).toBe(false)
    expect(vm.genError).toBe('boom')
    expect(refreshSpy).not.toHaveBeenCalled()
  })
})

/**
 * "Читать" CTA wiring (Task 18): DailyFanficCard links to /fanfics?daily=1,
 * and FanficsView opens the daily fanfic in the same reader Modal used by
 * the library grid — unless the pick is gated (explicit + not readable
 * here), in which case it surfaces a toast instead of an empty reader.
 * `readerOpen`/`readerFanfic`/`readerIsDaily` are exposed via defineExpose
 * specifically for these tests, same rationale as the streaming state above
 * (Modal/DialogPortal render through a Teleport, so DOM-level assertions
 * would need `attachTo: document.body`; asserting the underlying reactive
 * state is the more direct and equally faithful check).
 */
describe('FanficsView daily-fanfic deep link (?daily=1)', () => {
  beforeEach(() => {
    routeQueryRef.value = {}
    getDailyMock.mockReset()
    pushToastMock.mockReset()
    routerReplaceMock.mockReset()
    fanficVisibleHolder.value = true
  })

  const dailyFanfic = {
    id: 'daily-1',
    anime_id: '',
    anime_shikimori_id: '',
    anime_title: 'Frieren',
    anime_japanese: '',
    anime_poster: '',
    characters: [],
    tags: [],
    length: 'oneshot' as const,
    pov: 'third' as const,
    rating: 'teen' as const,
    language: 'ru' as const,
    prompt: '',
    title: 'Daily Title',
    content: 'Daily content body.',
    model: '',
    token_usage: 0,
    status: 'complete' as const,
    created_at: '2026-07-14T00:00:00Z',
    canon: false,
    part_count: 1,
  }

  it('does not call getDaily when the query has no daily=1', () => {
    mountView()
    expect(getDailyMock).not.toHaveBeenCalled()
  })

  it('opens the reader with the daily fanfic and marks it readerIsDaily, without a toast', async () => {
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockResolvedValueOnce({ ...dailyFanfic, gated: false })

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(getDailyMock).toHaveBeenCalledTimes(1)
    expect(vm.readerOpen).toBe(true)
    expect(vm.readerFanfic?.title).toBe('Daily Title')
    expect(vm.readerFanfic?.content).toBe('Daily content body.')
    expect(vm.readerIsDaily).toBe(true)
    expect(pushToastMock).not.toHaveBeenCalled()
  })

  it('an anonymous-gated pick (gate_reason:"login") surfaces a login toast and never opens the reader', async () => {
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockResolvedValueOnce({
      ...dailyFanfic,
      content: '',
      gated: true,
      gate_reason: 'login',
    })

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(vm.readerOpen).toBe(false)
    expect(vm.readerFanfic).toBeNull()
    expect(pushToastMock).toHaveBeenCalledWith('Log in to read today\'s fanfic.', 'info')
  })

  it('a logged-in-gated pick (gate_reason:"adult_setting") surfaces the explicit-gate toast, not the login one', async () => {
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockResolvedValueOnce({
      ...dailyFanfic,
      content: '',
      gated: true,
      gate_reason: 'adult_setting',
    })

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(vm.readerOpen).toBe(false)
    expect(pushToastMock).toHaveBeenCalledWith(
      'This pick is explicit (18+) and can\'t be opened here.',
      'info',
    )
  })

  it('a getDaily() rejection is caught and surfaced as an error toast, never throwing', async () => {
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockRejectedValueOnce(new Error('network down'))

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(vm.readerOpen).toBe(false)
    expect(pushToastMock).toHaveBeenCalledWith('Couldn\'t load today\'s fanfic.', 'error')
  })

  it('the library-grid open path (onOpenFanfic) leaves readerIsDaily false, so the Continue footer stays eligible', async () => {
    const { fanficApi } = await import('@/api/fanfic')
    vi.mocked(fanficApi.get).mockResolvedValueOnce({ ...dailyFanfic, id: 'lib-1' })

    const wrapper = mountView()
    const vm = wrapper.vm as unknown as FanficsViewVm & {
      onOpenFanfic: (id: string) => Promise<void>
    }
    await vm.onOpenFanfic('lib-1')

    expect(vm.readerOpen).toBe(true)
    expect(vm.readerFanfic?.id).toBe('lib-1')
    expect(vm.readerIsDaily).toBe(false)
  })

  // ── Reader-only viewers (fanfic feature NOT visible) ──────────────────────
  // The ?daily=1 deep link bypasses the route guard's fanfic gate, so users
  // without the feature can land here. They must get ONLY the daily reader:
  // no authoring tabs, and any exit (closing the dialog / a failed load)
  // routes them home instead of stranding them on an empty shell.

  it('reader-only viewer: authoring tabs are hidden, the daily reader opens, and closing it routes home', async () => {
    fanficVisibleHolder.value = false
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockResolvedValueOnce({ ...dailyFanfic, gated: false })

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(wrapper.find('[data-testid="generate-form-stub"]').exists()).toBe(false)
    expect(vm.readerOpen).toBe(true)

    vm.readerOpen = false
    await flushPromises()
    expect(routerReplaceMock).toHaveBeenCalledWith({ name: 'home' })
  })

  it('reader-only viewer: a failed daily load toasts and routes home', async () => {
    fanficVisibleHolder.value = false
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockRejectedValueOnce(new Error('network down'))

    mountView()
    await flushPromises()

    expect(pushToastMock).toHaveBeenCalledWith('Couldn\'t load today\'s fanfic.', 'error')
    expect(routerReplaceMock).toHaveBeenCalledWith({ name: 'home' })
  })

  it('full-feature viewer: closing the daily reader stays on /fanfics (no redirect)', async () => {
    routeQueryRef.value = { daily: '1' }
    getDailyMock.mockResolvedValueOnce({ ...dailyFanfic, gated: false })

    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as FanficsViewVm

    expect(wrapper.find('[data-testid="generate-form-stub"]').exists()).toBe(true)
    vm.readerOpen = false
    await flushPromises()
    expect(routerReplaceMock).not.toHaveBeenCalled()
  })
})
