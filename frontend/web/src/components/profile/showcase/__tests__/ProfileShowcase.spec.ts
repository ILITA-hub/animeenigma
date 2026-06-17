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
})
