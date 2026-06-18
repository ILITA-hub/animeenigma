import { mount } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import ShowcaseConfigDialog from '../ShowcaseConfigDialog.vue'
import type { ShowcaseBlock } from '@/types/showcase'

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
          ],
        },
      },
    }),
  },
}))

describe('ShowcaseConfigDialog', () => {
  it('re-clamps size when switching to a smaller-bound variant', async () => {
    const block: ShowcaseBlock = { type: 'favorite_anime', variant: 'grid', order: 0, w: 4, h: 3, config: { anime_ids: [] } }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' }, global: { mocks: { $t: (k: string) => k } } })
    await wrapper.find('[data-test="variant-podium"]').trigger('click') // podium fixed 2x2
    const updated = wrapper.emitted('update:block')!.at(-1)![0] as typeof block
    expect([updated.variant, updated.w, updated.h]).toEqual(['podium', 2, 2])
  })

  it('seeds config from props.block.config (persistence fix)', async () => {
    const block: ShowcaseBlock = { type: 'about', variant: 'bio', order: 0, w: 2, h: 2, config: { title: 'My Title', text: 'My Text' } }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' }, global: { mocks: { $t: (k: string) => k } } })
    const titleInput = wrapper.find('[data-test="about-title"]')
    expect((titleInput.element as HTMLInputElement).value).toBe('My Title')
  })

  it('emits close when close button clicked', async () => {
    const block: ShowcaseBlock = { type: 'stats', variant: 'tiles', order: 0, w: 2, h: 1, config: {} }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' }, global: { mocks: { $t: (k: string) => k } } })
    await wrapper.find('[data-test="dialog-close"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('auto-fill anime fills anime_ids and emits update:block', async () => {
    const { userApi } = await import('@/api/client')
    const block: ShowcaseBlock = { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' }, global: { mocks: { $t: (k: string) => k } } })
    await wrapper.find('[data-test="dialog-auto-anime"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    expect(userApi.getWatchlist).toHaveBeenCalled()
    const emitted = wrapper.emitted('update:block')
    expect(emitted).toBeTruthy()
    const updated = emitted!.at(-1)![0] as { config: { anime_ids: string[] } }
    expect(updated.config.anime_ids).toContain('a1')
  })

  it('op_ed add/remove theme updates config and emits update:block', async () => {
    const block: ShowcaseBlock = { type: 'op_ed', variant: 'grid', order: 0, w: 2, h: 2, config: { theme_ids: ['42'] } }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' }, global: { mocks: { $t: (k: string) => k } } })
    // Remove the existing theme
    await wrapper.find('[data-test="dialog-remove-theme-42"]').trigger('click')
    const emitted = wrapper.emitted('update:block')
    expect(emitted).toBeTruthy()
    const updated = emitted!.at(-1)![0] as { config: { theme_ids: string[] } }
    expect(updated.config.theme_ids).not.toContain('42')
  })
})
