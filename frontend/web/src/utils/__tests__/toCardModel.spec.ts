import { describe, it, expect, vi } from 'vitest'

// getLocalizedTitle/getLocalizedGenre read the active i18n locale; stub them to
// deterministic identity so the mappers are tested in isolation.
vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, ru?: string, jp?: string) => name || ru || jp || '',
  getLocalizedGenre: (name?: string, ru?: string) => name || ru || '',
}))

import { fromCatalogAnime, fromHomeAnime, fromContinueWatching } from '../toCardModel'

describe('fromCatalogAnime', () => {
  const base = {
    id: 'a1',
    title: 'Fallback',
    name: 'Frieren',
    coverImage: 'http://x/p.jpg',
    rating: 8.9,
    releaseYear: 2023,
    totalEpisodes: 28,
    episodesAired: 28,
    rawGenres: [{ name: 'Adventure' }],
    status: 'released',
    hasVideo: true,
    description: '',
    genres: [],
  }

  it('maps intrinsic fields', () => {
    const m = fromCatalogAnime(base as never)
    expect(m.id).toBe('a1')
    expect(m.href).toBe('/anime/a1')
    expect(m.title).toBe('Frieren')
    expect(m.coverImage).toBe('http://x/p.jpg')
    expect(m.year).toBe(2023)
    expect(m.episodes).toBe(28)
    expect(m.primaryGenre).toBe('Adventure')
    expect(m.malScore).toBe(8.9)
  })

  it('merges per-user extras', () => {
    const m = fromCatalogAnime(base as never, {
      siteScore: 9.4,
      listStatus: 'watching',
      progress: { current: 12, total: 28 },
    })
    expect(m.siteScore).toBe(9.4)
    expect(m.listStatus).toBe('watching')
    expect(m.progress).toEqual({ current: 12, total: 28 })
  })

  it('flags airing only for ongoing status', () => {
    const ongoing = fromCatalogAnime({ ...base, status: 'ongoing', nextEpisodeAt: '2026-06-10T12:00:00Z', episodesAired: 5 } as never)
    expect(ongoing.airing).toBe(true)
    expect(ongoing.nextEpisode).toEqual({ ep: 6, when: '2026-06-10T12:00:00Z' })
    expect(fromCatalogAnime(base as never).airing).toBe(false)
  })
})

describe('fromHomeAnime', () => {
  const h = {
    id: 'h1',
    name: 'Bocchi',
    poster_url: 'http://x/h.jpg',
    score: 8.2,
    episodes_count: 12,
    episodes_aired: 3,
    year: 2022,
    status: 'ongoing',
    next_episode_at: '2026-06-12T15:00:00Z',
  }

  it('maps fields and derives next episode', () => {
    const m = fromHomeAnime(h as never)
    expect(m.id).toBe('h1')
    expect(m.title).toBe('Bocchi')
    expect(m.coverImage).toBe('http://x/h.jpg')
    expect(m.episodes).toBe(12)
    expect(m.malScore).toBe(8.2)
    expect(m.airing).toBe(true)
    expect(m.nextEpisode).toEqual({ ep: 4, when: '2026-06-12T15:00:00Z' })
  })

  it('falls back to /placeholder.svg when poster missing', () => {
    const m = fromHomeAnime({ ...h, poster_url: undefined } as never)
    expect(m.coverImage).toBe('/placeholder.svg')
  })
})

describe('fromContinueWatching', () => {
  const item = {
    anime: { id: 'c1', name: 'Dandadan', poster_url: 'http://x/c.jpg', episodes_count: 12 },
    episode_number: 5,
    progress: 600,
    duration: 1400,
  }

  it('builds an episode-deep href and progress', () => {
    const m = fromContinueWatching(item as never)
    expect(m.id).toBe('c1')
    expect(m.title).toBe('Dandadan')
    expect(m.href).toBe('/anime/c1?episode=5')
    expect(m.nextEpisode).toEqual({ ep: 5, when: '' })
    expect(m.progress).toEqual({ current: 5, total: 12 })
  })
})
