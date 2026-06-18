import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AnimeDnaBlock from '../AnimeDnaBlock.vue'

vi.mock('@/api/client', () => ({
  publicApi: {
    getPublicWatchlistFacets: vi.fn().mockResolvedValue({
      data: {
        genres: [
          { id: '1', name: 'Action', name_ru: 'Экшен', count: 30 },
          { id: '2', name: 'Romance', name_ru: 'Романтика', count: 20 },
          { id: '3', name: 'Comedy', name_ru: 'Комедия', count: 15 },
        ],
        kinds: [],
        years: { min: null, max: null },
      },
    }),
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale: { value: 'en' } }),
}))

describe('AnimeDnaBlock', () => {
  it('renders one bar per genre facet', async () => {
    const w = mount(AnimeDnaBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    const bars = w.findAll('[data-testid="dna-bar"]')
    expect(bars.length).toBe(3)
  })

  it('renders genre names', async () => {
    const w = mount(AnimeDnaBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    const names = w.findAll('[data-testid="dna-genre-name"]')
    expect(names[0].text()).toContain('Action')
    expect(names[1].text()).toContain('Romance')
  })

  it('renders nothing when genres are empty', async () => {
    const { publicApi } = await import('@/api/client')
    vi.mocked(publicApi.getPublicWatchlistFacets).mockResolvedValueOnce({
      data: { genres: [], kinds: [], years: { min: null, max: null } },
    } as never)
    const w = mount(AnimeDnaBlock, {
      props: { userId: 'user-empty' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.findAll('[data-testid="dna-bar"]').length).toBe(0)
  })

  it('renders nothing when API throws', async () => {
    const { publicApi } = await import('@/api/client')
    vi.mocked(publicApi.getPublicWatchlistFacets).mockRejectedValueOnce(new Error('fail'))
    const w = mount(AnimeDnaBlock, {
      props: { userId: 'user-err' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.findAll('[data-testid="dna-bar"]').length).toBe(0)
  })

  it('shows the block title i18n key', async () => {
    const w = mount(AnimeDnaBlock, {
      props: { userId: 'user-1' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.text()).toContain('showcase.block.anime_dna')
  })
})
