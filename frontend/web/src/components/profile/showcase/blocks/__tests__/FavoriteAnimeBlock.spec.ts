import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FavoriteAnimeBlock from '../FavoriteAnimeBlock.vue'

// Mock api so onMounted no-ops on empty config
vi.mock('@/api/client', () => ({
  animeApi: {
    getById: vi.fn().mockResolvedValue({ data: {} }),
  },
}))

// Mock PosterCard to avoid deep dependency chain in tests
vi.mock('@/components/anime/PosterCard.vue', () => ({
  default: { template: '<div data-testid="poster-card" />' },
}))

vi.mock('@/utils/toCardModel', () => ({
  fromHomeAnime: vi.fn((a) => ({ id: a.id ?? '', href: `/anime/${a.id ?? ''}`, title: '', coverImage: '' })),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: vi.fn((n) => n ?? ''),
  getLocalizedGenre: vi.fn((n) => n ?? ''),
}))

vi.mock('@/composables/useTitleLang', () => ({
  useTitleLang: () => ({ titleLang: { value: 'ru' } }),
}))

describe('FavoriteAnimeBlock', () => {
  it('mounts without crashing', () => {
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the favorite anime title key', () => {
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.favorite_anime')
  })
})
