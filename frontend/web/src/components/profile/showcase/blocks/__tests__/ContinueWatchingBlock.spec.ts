import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ContinueWatchingBlock from '../ContinueWatchingBlock.vue'

vi.mock('@/api/client', () => ({
  publicApi: {
    getPublicWatchlist: vi.fn().mockResolvedValue({
      data: [
        { anime_id: '1', anime: { name: 'Naruto', name_ru: 'Наруто', poster_url: '/p1.jpg' } },
        { anime_id: '2', anime: { name: 'Bleach', name_ru: 'Блич', poster_url: '/p2.jpg' } },
      ],
    }),
  },
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: vi.fn((name: string) => name ?? ''),
}))

vi.mock('@/composables/useImageProxy', () => ({
  getImageUrl: vi.fn((url: string) => url ?? ''),
  // PosterImage (child) resizes/falls back through these.
  cardPosterUrl: (url: string) => url,
  getImageFallbackUrl: (url: string) => url,
}))

describe('ContinueWatchingBlock', () => {
  it('renders one card per watching entry', async () => {
    const w = mount(ContinueWatchingBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    const cards = w.findAll('[data-testid="cw-card"]')
    expect(cards.length).toBe(2)
  })

  it('renders the anime title for each entry', async () => {
    const w = mount(ContinueWatchingBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    const titles = w.findAll('[data-testid="cw-title"]')
    expect(titles[0].text()).toContain('Naruto')
    expect(titles[1].text()).toContain('Bleach')
  })

  it('renders nothing when the list is empty', async () => {
    const { publicApi } = await import('@/api/client')
    vi.mocked(publicApi.getPublicWatchlist).mockResolvedValueOnce({ data: [] } as never)
    const w = mount(ContinueWatchingBlock, {
      props: { userId: 'user-empty' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.findAll('[data-testid="cw-card"]').length).toBe(0)
  })

  it('renders nothing when API throws', async () => {
    const { publicApi } = await import('@/api/client')
    vi.mocked(publicApi.getPublicWatchlist).mockRejectedValueOnce(new Error('fail'))
    const w = mount(ContinueWatchingBlock, {
      props: { userId: 'user-err' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.findAll('[data-testid="cw-card"]').length).toBe(0)
  })

  it('shows the block title i18n key', async () => {
    const w = mount(ContinueWatchingBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.text()).toContain('showcase.block.continue_watching')
  })
})
