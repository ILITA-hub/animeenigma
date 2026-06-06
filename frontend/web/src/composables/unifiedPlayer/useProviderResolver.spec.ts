import { describe, it, expect, vi } from 'vitest'
import { makeResolver } from './useProviderResolver'

describe('useProviderResolver', () => {
  it('dispatches to the scraper adapter for an EN provider', async () => {
    const scraperApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: {
          data: {
            episodes: [{ id: 'e1', number: 1 }],
            meta: { provider: 'allanime' },
          },
        },
      }),
      getServers: vi.fn().mockResolvedValue({
        data: {
          data: {
            servers: [{ id: 's1', name: 'Server 1' }],
          },
        },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: {
          data: {
            stream: {
              sources: [{ url: 'http://x/m3u8', type: 'hls' }],
            },
          },
        },
      }),
    }
    const resolver = makeResolver({ scraperApi } as any)
    const eps = await resolver.listEpisodes('allanime', 'anime-uuid')
    expect(eps[0].number).toBe(1)
    const stream = await resolver.resolveStream('allanime', 'anime-uuid', eps[0], {
      audio: 'sub',
      lang: 'en',
      provider: 'allanime',
      server: 's1',
      team: null,
    })
    expect(stream.type).toBe('hls')
    expect(scraperApi.getEpisodes).toHaveBeenCalledWith('anime-uuid', 'allanime')
  })

  it('throws a typed error for a disabled/unwired provider', async () => {
    const resolver = makeResolver({} as any)
    await expect(resolver.listEpisodes('animelib', 'x')).rejects.toThrow(/not available/i)
  })
})
