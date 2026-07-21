import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn() },
  animeApi: { getPopular: vi.fn() },
}))

import { animeApi, apiClient } from '@/api/client'
import { resolvePlayerGuideAnimeId } from './siteGuideState'

describe('resolvePlayerGuideAnimeId', () => {
  beforeEach(() => vi.clearAllMocks())

  it('uses the current Curator Recommends anime', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      data: {
        cards: [{ type: 'curated', data: { anime: { id: 'curator-anime', has_video: true } } }],
        generated_at: '2026-07-21T00:00:00Z',
      },
    } as never)

    await expect(resolvePlayerGuideAnimeId()).resolves.toBe('curator-anime')
    expect(animeApi.getPopular).not.toHaveBeenCalled()
  })

  it('falls back to popular top-1 when Curator Recommends is unavailable', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      data: { data: { cards: [], generated_at: '2026-07-21T00:00:00Z' } },
    } as never)
    vi.mocked(animeApi.getPopular).mockResolvedValue({
      data: { data: [{ id: 'popular-top-1' }, { id: 'popular-top-2' }] },
    } as never)

    await expect(resolvePlayerGuideAnimeId()).resolves.toBe('popular-top-1')
  })
})
