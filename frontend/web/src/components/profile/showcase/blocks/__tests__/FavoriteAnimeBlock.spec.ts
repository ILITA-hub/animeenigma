import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FavoriteAnimeBlock from '../FavoriteAnimeBlock.vue'

const mockAnime = (id: string, score: number) => ({
  id,
  name: `Anime ${id}`,
  name_ru: `Аниме ${id}`,
  poster_url: `https://example.com/${id}.jpg`,
  score,
  episodes_count: 24,
})

const getByIdMock = vi.fn()

vi.mock('@/api/client', () => ({
  animeApi: {
    getById: (...args: unknown[]) => getByIdMock(...args),
  },
}))

// Mock PosterCard to avoid deep dependency chain in tests
vi.mock('@/components/anime/PosterCard.vue', () => ({
  default: { template: '<div data-testid="poster-card" />' },
}))

vi.mock('@/utils/toCardModel', () => ({
  fromHomeAnime: vi.fn((a) => ({
    id: a.id ?? '',
    href: `/anime/${a.id ?? ''}`,
    title: a.name ?? '',
    coverImage: a.poster_url ?? '',
    malScore: a.score,
    episodes: a.episodes_count,
  })),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: vi.fn((n) => n ?? ''),
  getLocalizedGenre: vi.fn((n) => n ?? ''),
}))

vi.mock('@/composables/useTitleLang', () => ({
  useTitleLang: () => ({ titleLang: { value: 'ru' } }),
}))

describe('FavoriteAnimeBlock', () => {
  beforeEach(() => {
    getByIdMock.mockReset()
  })

  it('mounts without crashing', () => {
    getByIdMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the favorite anime title key', () => {
    getByIdMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.favorite_anime')
  })

  it('default variant (row) renders PosterCard for each resolved anime', async () => {
    getByIdMock.mockImplementation((_id: string) =>
      Promise.resolve({ data: { data: mockAnime(_id, 9.0) } }),
    )
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1', '2', '3'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const cards = w.findAll('[data-testid="poster-card"]')
    expect(cards.length).toBe(3)
  })

  it('podium variant renders 3 ranked PosterCards in olympic order (2nd | 1st | 3rd)', async () => {
    getByIdMock.mockImplementation((_id: string) => {
      const scores: Record<string, number> = { '1': 9.2, '2': 9.0, '3': 8.9 }
      return Promise.resolve({ data: { data: mockAnime(_id, scores[_id] ?? 8.0) } })
    })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1', '2', '3'] }, variant: 'podium' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    // 3 PosterCards rendered (one per podium slot)
    const cards = w.findAll('[data-testid="poster-card"]')
    expect(cards.length).toBe(3)
    // olympic order: rank 2 first, rank 1 second, rank 3 third
    const slots = w.findAll('[data-rank]')
    expect(slots[0].attributes('data-rank')).toBe('2')
    expect(slots[1].attributes('data-rank')).toBe('1')
    expect(slots[2].attributes('data-rank')).toBe('3')
    // crown shown for 1st place
    expect(w.text()).toContain('👑')
  })

  it('podium variant: only 1 item renders without crash (no silver/bronze)', async () => {
    getByIdMock.mockImplementation((_id: string) =>
      Promise.resolve({ data: { data: mockAnime(_id, 9.0) } }),
    )
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1'] }, variant: 'podium' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.findAll('[data-testid="poster-card"]').length).toBe(1)
  })

  it('grid variant renders PosterCards in a grid', async () => {
    getByIdMock.mockImplementation((_id: string) =>
      Promise.resolve({ data: { data: mockAnime(_id, 8.5) } }),
    )
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1', '2', '3', '4', '5', '6'] }, variant: 'grid' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.findAll('[data-testid="poster-card"]').length).toBe(6)
  })

  it('list variant renders rows with titles and scores', async () => {
    getByIdMock.mockImplementation((_id: string) => {
      const names: Record<string, string> = { '1': 'Re:Zero', '2': 'Steins;Gate', '3': 'Monogatari' }
      const scores: Record<string, number> = { '1': 9.2, '2': 9.0, '3': 8.9 }
      const a = { ...mockAnime(_id, scores[_id] ?? 8.0), name: names[_id] ?? `Anime ${_id}` }
      return Promise.resolve({ data: { data: a } })
    })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1', '2', '3'] }, variant: 'list' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const rows = w.findAll('[data-testid="list-title"]')
    expect(rows.length).toBe(3)
    expect(rows[0].text()).toBe('Re:Zero')
    expect(rows[1].text()).toBe('Steins;Gate')
    // score shown for first item
    expect(w.text()).toContain('◆ 9.2')
    expect(w.text()).toContain('◆ 9.0')
  })

  it('banner variant renders wide covers with titles', async () => {
    getByIdMock.mockImplementation((_id: string) => {
      const names: Record<string, string> = { '1': 'Re:Zero', '2': 'Frieren' }
      const a = { ...mockAnime(_id, 9.0), name: names[_id] ?? `Anime ${_id}` }
      return Promise.resolve({ data: { data: a } })
    })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: ['1', '2'] }, variant: 'banner' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const banners = w.findAll('[data-testid="banner-title"]')
    expect(banners.length).toBe(2)
    expect(banners[0].text()).toBe('Re:Zero')
    expect(banners[1].text()).toBe('Frieren')
  })

  it('shows empty message when no ids provided', () => {
    getByIdMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteAnimeBlock, {
      props: { config: { anime_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.empty')
  })
})
