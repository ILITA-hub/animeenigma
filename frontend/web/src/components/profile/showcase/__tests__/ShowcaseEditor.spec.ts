import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ShowcaseEditor from '../ShowcaseEditor.vue'
import type { ShowcaseBlock } from '@/types/showcase'
import { defaultVariant } from '@/types/showcase'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: vi.fn() }),
}))

// ShowcaseConfigDialog (imported by ShowcaseEditor) transitively pulls in
// @/api/client and @/api/gacha — mock them to avoid the auth/i18n chain.
vi.mock('@/api/client', () => ({
  userApi: { getWatchlist: vi.fn() },
}))

vi.mock('@/api/gacha', () => ({
  gachaApi: { getCollection: vi.fn() },
}))

const blocks: ShowcaseBlock[] = [
  { type: 'about', order: 0, config: { text: 'hi' } },
  { type: 'stats', order: 1, config: {} },
]

const mountEditor = (modelValue: ShowcaseBlock[] = blocks) =>
  mount(ShowcaseEditor, {
    props: { userId: 'u1', modelValue },
    global: {
      mocks: { $t: (k: string) => k },
      stubs: { teleport: true, Select: true, ShowcaseBlockView: true, ShowcaseConfigDialog: true },
    },
  })

describe('ShowcaseEditor', () => {
  it('renders one grid cell per block', () => {
    const w = mountEditor()
    // Each block renders a draggable grid cell with a remove control.
    expect(w.findAll('[data-test^="showcase-remove-"]')).toHaveLength(blocks.length)
  })

  it('emits save with re-numbered order on save', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-save"]').trigger('click')
    const emitted = w.emitted('save')
    expect(emitted).toBeTruthy()
    const payload = emitted![0][0] as ShowcaseBlock[]
    expect(payload.map((b) => b.order)).toEqual([0, 1])
  })

  it('removes a block', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-remove-0"]').trigger('click')
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect(payload).toHaveLength(1)
  })

  it('adding a block sets variant to defaultVariant(type)', async () => {
    const w = mountEditor([])
    // Open the picker first, then click the favorite_anime option
    await w.find('[data-test="showcase-open-picker"]').trigger('click')
    await w.vm.$nextTick()
    const animeBtn = w.find('[data-test="picker-favorite_anime"]')
    expect(animeBtn.exists()).toBeTruthy()
    await animeBtn.trigger('click')
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect(payload).toHaveLength(1)
    expect(payload[0].variant).toBe(defaultVariant('favorite_anime'))
  })

  it('save payload includes variant on every block', async () => {
    const customBlocks: ShowcaseBlock[] = [
      { type: 'favorite_anime', order: 0, variant: 'podium', config: { anime_ids: [] } },
      { type: 'stats', order: 1, config: {} },
    ]
    const w = mountEditor(customBlocks)
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect(payload[0].variant).toBe('podium')
    // stats has only 1 variant so save should fill the default
    expect(payload[1].variant).toBe(defaultVariant('stats'))
  })

  it('each block cell has a ⚙ config button', () => {
    const w = mountEditor()
    expect(w.find('[data-test="showcase-config-0"]').exists()).toBe(true)
    expect(w.find('[data-test="showcase-config-1"]').exists()).toBe(true)
  })

  it('clicking ⚙ opens the config dialog (sets configIdx)', async () => {
    const w = mountEditor()
    expect((w.vm as any).configIdx).toBeNull()
    await w.find('[data-test="showcase-config-0"]').trigger('click')
    expect((w.vm as any).configIdx).toBe(0)
  })

  it('closing the dialog resets configIdx', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-config-1"]').trigger('click')
    expect((w.vm as any).configIdx).toBe(1)
    ;(w.vm as any).closeConfig()
    expect((w.vm as any).configIdx).toBeNull()
  })

  it('onBlockUpdate applies dialog update:block payload', async () => {
    const customBlocks: ShowcaseBlock[] = [
      { type: 'about', order: 0, config: { title: '', text: '' } },
    ]
    const w = mountEditor(customBlocks)
    await w.find('[data-test="showcase-config-0"]').trigger('click')
    const updated: ShowcaseBlock = { type: 'about', order: 0, variant: 'bio', config: { title: 'New Title', text: 'hello' } }
    ;(w.vm as any).onBlockUpdate(updated)
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect((payload[0].config as { title: string }).title).toBe('New Title')
  })

  it('no inline auto-fill buttons remain in the editor (config is dialog-only)', () => {
    const customBlocks: ShowcaseBlock[] = [
      { type: 'favorite_anime', order: 0, variant: 'row', config: { anime_ids: [] } },
      { type: 'card_collection', order: 1, config: { card_ids: [] } },
    ]
    const w = mountEditor(customBlocks)
    // Old inline auto buttons must be gone — config lives exclusively in the dialog
    expect(w.find('[data-test="showcase-auto-anime-0"]').exists()).toBe(false)
    expect(w.find('[data-test="showcase-auto-cards-1"]').exists()).toBe(false)
  })

  it('no inline op_ed theme input remains in the editor', () => {
    const customBlocks: ShowcaseBlock[] = [
      { type: 'op_ed', order: 0, config: { theme_ids: ['t1'] } },
    ]
    const w = mountEditor(customBlocks)
    expect(w.find('[data-test="showcase-theme-input"]').exists()).toBe(false)
    expect(w.find('[data-test="showcase-theme-add"]').exists()).toBe(false)
  })

  it('add menu lists 4 new block types', async () => {
    const w = mountEditor([])
    // Open the picker to see the block type options
    ;(w.vm as any).pickerOpen = true
    await w.vm.$nextTick()
    const text = w.text()
    expect(text).toContain('showcase.block.continue_watching')
    expect(text).toContain('showcase.block.op_ed')
    expect(text).toContain('showcase.block.anime_dna')
    expect(text).toContain('showcase.block.compatibility')
  })

  it('resize clamps to the variant bounds', () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
      { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
    ] }, global: { stubs: { draggable: true, Select: true, ShowcaseBlockView: true, ShowcaseConfigDialog: true }, mocks: { $t: (k: string) => k } } })
    ;(wrapper.vm as any).applyResize(0, -5, +2) // try to shrink below min / grow past max-h(1)
    const b = (wrapper.vm as any).local[0]
    expect([b.w, b.h]).toEqual([2, 1]) // row: W2..4, H fixed 1
  })

  it('reports fixed-size variants', () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
      { type: 'stats', variant: 'tiles', order: 0, w: 2, h: 1, config: {} },
    ] }, global: { stubs: { draggable: true, Select: true, ShowcaseBlockView: true, ShowcaseConfigDialog: true }, mocks: { $t: (k: string) => k } } })
    expect((wrapper.vm as any).isFixed((wrapper.vm as any).local[0])).toBe(true)
  })

  // ── Visibility toggle + nudge (opt-in showcase) ─────────────────────────
  it('renders the visibility toggle', () => {
    const w = mountEditor()
    expect(w.find('[data-test="showcase-visible-toggle"]').exists()).toBe(true)
  })

  it('disables the visibility toggle when there are 0 blocks', () => {
    const w = mountEditor([])
    const toggle = w.find('[data-test="showcase-visible-toggle"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('disabled')).toBeDefined()
  })

  it('save emits [blocks, enabled] — enabled defaults false', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-save"]').trigger('click')
    const emitted = w.emitted('save')
    expect(emitted).toBeTruthy()
    expect(emitted![0][1]).toBe(false)
  })

  it('save emits enabled=true when the editor is mounted enabled', async () => {
    const w = mount(ShowcaseEditor, {
      props: { userId: 'u1', modelValue: blocks, enabled: true },
      global: { mocks: { $t: (k: string) => k }, stubs: { teleport: true, Select: true, ShowcaseBlockView: true, ShowcaseConfigDialog: true } },
    })
    await w.find('[data-test="showcase-save"]').trigger('click')
    expect(w.emitted('save')![0][1]).toBe(true)
  })

  it('shows the hidden nudge after saving content while disabled', async () => {
    const w = mountEditor()
    expect(w.find('[data-test="showcase-hidden-nudge"]').exists()).toBe(false)
    await w.find('[data-test="showcase-save"]').trigger('click')
    expect(w.find('[data-test="showcase-hidden-nudge"]').exists()).toBe(true)
  })

  it('Enable-now in the nudge re-saves with enabled=true', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-save"]').trigger('click')
    await w.find('[data-test="showcase-enable-now"]').trigger('click')
    const emitted = w.emitted('save')!
    // first save = hidden, second save (from Enable) = enabled
    expect(emitted[0][1]).toBe(false)
    expect(emitted[1][1]).toBe(true)
    // nudge dismissed after enabling
    expect(w.find('[data-test="showcase-hidden-nudge"]').exists()).toBe(false)
  })

  it('no nudge when saving an empty showcase', async () => {
    const w = mountEditor([])
    await w.find('[data-test="showcase-save"]').trigger('click')
    expect(w.find('[data-test="showcase-hidden-nudge"]').exists()).toBe(false)
  })

  it('disables already-present types in the picker', async () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
      { type: 'about', variant: 'bio', order: 0, w: 2, h: 2, config: { title: '', text: '' } },
    ] }, global: { stubs: { draggable: true, Select: true, ShowcaseBlockView: true, ShowcaseConfigDialog: true }, mocks: { $t: (k: string) => k } } })
    ;(wrapper.vm as any).pickerOpen = true
    await wrapper.vm.$nextTick()
    const aboutBtn = wrapper.find('[data-test="picker-about"]')
    expect(aboutBtn.attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="picker-stats"]').attributes('disabled')).toBeUndefined()
  })
})
