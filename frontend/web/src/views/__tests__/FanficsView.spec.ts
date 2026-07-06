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
import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import FanficsView from '../FanficsView.vue'
import type { GenerateInput, StreamHandlers } from '@/types/fanfic'

// vi.mock factories are hoisted above imports — anything they close over
// must itself be created via vi.hoisted() (see GenerateForm.spec.ts).
const { generateMock, releaseGenerate } = vi.hoisted(() => {
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
  return { generateMock, releaseGenerate: () => release?.() }
})

vi.mock('@/api/fanfic', () => ({
  fanficApi: {
    generate: generateMock,
    list: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    get: vi.fn(),
    remove: vi.fn(),
    tags: vi.fn().mockResolvedValue([]),
  },
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
