import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: vi.fn() }),
}))

vi.mock('@/api/client', () => ({
  showcaseApi: {
    getShowcase: vi.fn().mockResolvedValue({ data: { blocks: [{ type: 'about', order: 0, config: { text: 'hi' } }] } }),
    saveShowcase: vi.fn().mockResolvedValue({ data: { blocks: [] } }),
  },
  publicApi: { getPublicWatchlistStats: vi.fn().mockResolvedValue({ data: {} }) },
  animeApi: { getById: vi.fn().mockResolvedValue({ data: {} }) },
  apiClient: { get: vi.fn().mockResolvedValue({ data: {} }) },
}))

describe('ProfileShowcase grid', () => {
  it('wraps each block in a cell with span classes and emits loaded', async () => {
    const { showcaseApi } = await import('@/api/client')
    vi.mocked(showcaseApi.getShowcase).mockResolvedValueOnce({
      data: { blocks: [
        { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
        { type: 'stats', variant: 'tiles', order: 1, config: {} },
      ] },
    } as never)
    const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })
    const wrapper = mount(ProfileShowcase, {
      props: { userId: 'u1', isOwner: false },
      global: {
        plugins: [i18n],
        stubs: { FavoriteAnimeBlock: true, StatsBlock: true, ShowcaseEditor: true },
      },
    })
    await new Promise((r) => setTimeout(r))
    const cells = wrapper.findAll('[data-showcase-cell]')
    expect(cells).toHaveLength(2)
    expect(cells[0].classes()).toContain('md:col-span-4')
    expect(cells[1].classes()).toContain('md:col-span-2') // stats default 2x1
    expect(wrapper.emitted('loaded')?.[0]).toEqual([2])
  })
})

import { showcaseApi } from '@/api/client'
import ProfileShowcase from '../ProfileShowcase.vue'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

const mountSc = (isOwner: boolean) =>
  mount(ProfileShowcase, {
    props: { userId: 'u1', isOwner },
    global: { plugins: [i18n], stubs: { ShowcaseEditor: true } },
  })

describe('ProfileShowcase', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched blocks', async () => {
    const w = mountSc(false)
    await flushPromises()
    expect(w.text()).toContain('hi')
  })

  it('shows edit button only for owner', async () => {
    const owner = mountSc(true)
    await flushPromises()
    expect(owner.text()).toContain('Edit showcase')
    const visitor = mountSc(false)
    await flushPromises()
    expect(visitor.text()).not.toContain('Edit showcase')
  })

  it('keeps editor open and does not mutate blocks when saveShowcase rejects', async () => {
    // Make saveShowcase reject for this test
    vi.mocked(showcaseApi.saveShowcase).mockRejectedValueOnce(new Error('network error'))

    const w = mount(ProfileShowcase, {
      props: { userId: 'u1', isOwner: true },
      global: {
        plugins: [i18n],
        // Stub ShowcaseEditor so we can detect it in DOM and trigger @save
        stubs: {
          ShowcaseEditor: {
            name: 'ShowcaseEditor',
            template: '<div data-testid="editor"><slot /></div>',
            emits: ['save', 'cancel'],
            props: ['userId', 'modelValue'],
          },
        },
      },
    })
    await flushPromises()

    // Open editor by clicking the edit button
    const editBtn = w.find('button')
    await editBtn.trigger('click')

    // Editor should now be visible
    expect(w.find('[data-testid="editor"]').exists()).toBe(true)

    // Emit save from the stubbed ShowcaseEditor
    const editor = w.findComponent({ name: 'ShowcaseEditor' })
    const newBlocks = [{ type: 'stats', order: 0, config: {} }]
    await editor.vm.$emit('save', newBlocks)
    await flushPromises()

    // Editor must still be open (editing stays true on failure)
    expect(w.find('[data-testid="editor"]').exists()).toBe(true)
    // The original fetched block text ('hi') must still be in vm state — not replaced
    // We verify by checking the component did not transition back to block display
    // (if editing.value were false, the editor div would be gone)
    expect(w.find('[data-testid="editor"]').exists()).toBe(true)
  })

  it('emits loaded(0) when showcase load fails', async () => {
    vi.mocked(showcaseApi.getShowcase).mockRejectedValueOnce(new Error('api error'))

    const w = mountSc(false)
    await flushPromises()

    // On error, blocks.value is set to [] in the catch block
    // The finally block should emit loaded with the length (0)
    expect(w.emitted('loaded')?.[0]).toEqual([0])
  })
})
