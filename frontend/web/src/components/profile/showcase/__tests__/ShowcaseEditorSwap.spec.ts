import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ShowcaseEditor from '../ShowcaseEditor.vue'
import type { ShowcaseBlock } from '@/types/showcase'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: vi.fn() }),
}))

vi.mock('vuedraggable', () => ({
  default: {
    name: 'draggable',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<div><slot v-for="(el, idx) in modelValue" :element="el" :index="idx" name="item" /></div>',
  },
}))

vi.mock('@/api/client', () => ({
  userApi: { getWatchlist: vi.fn().mockResolvedValue({ data: { data: [] } }) },
}))

vi.mock('@/api/gacha', () => ({
  gachaApi: {
    getCollection: vi.fn().mockResolvedValue({ data: { data: { cards: [] } } }),
  },
}))

const blocks: ShowcaseBlock[] = [
  { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
  { type: 'compatibility', variant: 'ring', order: 1, w: 2, h: 1, config: {} },
]

describe('ShowcaseEditor swap', () => {
  it('swaps order and exchanges sizes clamped to each block\'s variant', async () => {
    const wrapper = mount(ShowcaseEditor, {
      props: { userId: 'u1', modelValue: blocks },
      global: {
        mocks: { $t: (k: string) => k },
        stubs: { teleport: true, Select: true, ShowcaseBlockView: true },
      },
    })
    ;(wrapper.vm as any).swapBlocks(0, 1)
    const local = (wrapper.vm as any).local as typeof blocks
    // anime gets compatibility's 2x1 -> clamped to anime/row (W2..4, H1) = 2x1
    // compatibility gets anime's 4x1 -> clamped to ring (W1..2, H1) = 2x1
    const anime = local.find((b) => b.type === 'favorite_anime')!
    const compat = local.find((b) => b.type === 'compatibility')!
    expect([anime.w, anime.h]).toEqual([2, 1])
    expect([compat.w, compat.h]).toEqual([2, 1])
  })

  it('does nothing if either index is out of bounds', () => {
    const wrapper = mount(ShowcaseEditor, {
      props: { userId: 'u1', modelValue: blocks },
      global: {
        mocks: { $t: (k: string) => k },
        stubs: { teleport: true, Select: true, ShowcaseBlockView: true },
      },
    })
    const before = JSON.stringify((wrapper.vm as any).local)
    ;(wrapper.vm as any).swapBlocks(0, 99)
    expect(JSON.stringify((wrapper.vm as any).local)).toBe(before)
  })

  it('swaps same-index is a no-op (block order preserved)', () => {
    const wrapper = mount(ShowcaseEditor, {
      props: { userId: 'u1', modelValue: blocks },
      global: {
        mocks: { $t: (k: string) => k },
        stubs: { teleport: true, Select: true, ShowcaseBlockView: true },
      },
    })
    ;(wrapper.vm as any).swapBlocks(0, 0)
    const local = (wrapper.vm as any).local as typeof blocks
    expect(local[0].type).toBe('favorite_anime')
  })
})
