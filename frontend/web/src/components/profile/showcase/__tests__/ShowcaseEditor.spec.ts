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

vi.mock('@/api/client', () => ({
  userApi: {
    getWatchlist: vi.fn().mockResolvedValue({
      data: {
        data: [
          { anime_id: 'a1', score: 9 },
          { anime_id: 'a2', score: 8 },
          { anime_id: 'a3', score: 7 },
        ],
      },
    }),
  },
}))

vi.mock('@/api/gacha', () => ({
  gachaApi: {
    getCollection: vi.fn().mockResolvedValue({
      data: {
        data: {
          cards: [
            { card: { id: 'c1', rarity: 'SSR', created_at: '2026-01-01' }, owned: true, count: 1 },
            { card: { id: 'c2', rarity: 'SR', created_at: '2026-01-02' }, owned: false, count: 0 },
            { card: { id: 'c3', rarity: 'R', created_at: '2026-01-03' }, owned: true, count: 1 },
          ],
        },
      },
    }),
  },
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
      stubs: { teleport: true, Select: true, ShowcaseBlockView: true },
    },
  })

describe('ShowcaseEditor', () => {
  it('renders one row per block', () => {
    const w = mountEditor()
    expect(w.text()).toContain('showcase.block.about')
    expect(w.text()).toContain('showcase.block.stats')
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
    // Click the "favorite_anime" add button
    const addButtons = w.findAll('button')
    const animeBtn = addButtons.find((b) => b.text().includes('showcase.block.favorite_anime'))
    expect(animeBtn).toBeTruthy()
    await animeBtn!.trigger('click')
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

  it('auto button on favorite_anime fills config.anime_ids', async () => {
    const { userApi } = await import('@/api/client')
    const customBlocks: ShowcaseBlock[] = [
      { type: 'favorite_anime', order: 0, variant: 'row', config: { anime_ids: [] } },
    ]
    const w = mountEditor(customBlocks)
    const autoBtn = w.find('[data-test="showcase-auto-anime-0"]')
    expect(autoBtn.exists()).toBe(true)
    await autoBtn.trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    expect(userApi.getWatchlist).toHaveBeenCalled()
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    const cfg = payload[0].config as { anime_ids: string[] }
    expect(cfg.anime_ids).toContain('a1')
    expect(cfg.anime_ids).toContain('a2')
  })

  it('add menu lists 4 new block types', () => {
    const w = mountEditor([])
    const text = w.text()
    expect(text).toContain('showcase.block.continue_watching')
    expect(text).toContain('showcase.block.op_ed')
    expect(text).toContain('showcase.block.anime_dna')
    expect(text).toContain('showcase.block.compatibility')
  })

  it('resize clamps to the variant bounds', () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
      { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
    ] }, global: { stubs: { draggable: true, Select: true, ShowcaseBlockView: true }, mocks: { $t: (k: string) => k } } })
    ;(wrapper.vm as any).applyResize(0, -5, +2) // try to shrink below min / grow past max-h(1)
    const b = (wrapper.vm as any).local[0]
    expect([b.w, b.h]).toEqual([2, 1]) // row: W2..4, H fixed 1
  })

  it('reports fixed-size variants', () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
      { type: 'stats', variant: 'tiles', order: 0, w: 2, h: 1, config: {} },
    ] }, global: { stubs: { draggable: true, Select: true, ShowcaseBlockView: true }, mocks: { $t: (k: string) => k } } })
    expect((wrapper.vm as any).isFixed((wrapper.vm as any).local[0])).toBe(true)
  })
})
