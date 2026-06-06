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

  it('routes 18anime to the anime18 adapter (NOT the scraper)', async () => {
    const scraperApi = { getEpisodes: vi.fn(), getServers: vi.fn(), getStream: vi.fn() }
    const anime18Api = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: [{ slug: 'ep-1', number: 1 }] },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: { url: 'http://x/h.m3u8', is_hls: true, quality: '720p' } },
      }),
    }
    const resolver = makeResolver({ scraperApi, anime18Api } as any)
    const eps = await resolver.listEpisodes('18anime', 'uuid')
    expect(anime18Api.getEpisodes).toHaveBeenCalledWith('uuid')
    expect(scraperApi.getEpisodes).not.toHaveBeenCalled()
    expect(eps.length).toBe(1)
    expect(eps[0].key).toBe('ep-1')
    expect(eps[0].number).toBe(1)
    const stream = await resolver.resolveStream('18anime', 'uuid', eps[0], {
      audio: 'sub',
      lang: 'en',
      provider: '18anime',
      server: '',
      team: null,
    })
    expect(anime18Api.getStream).toHaveBeenCalledWith('uuid', 'ep-1')
    expect(scraperApi.getStream).not.toHaveBeenCalled()
    expect(stream.url).toBe('http://x/h.m3u8')
    expect(stream.type).toBe('hls')
  })

  it('wires kodik via translations + proxy-wrapped stream', async () => {
    const kodikApi = {
      getTranslations: vi.fn().mockResolvedValue({ data: { data: [{ id: 7, title: 'AniLibria', type: 'voice', episodes_count: 3 }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { data: { stream_url: 'http://cdn/x.m3u8', referer: 'https://kodik' } } }),
    }
    const resolver = makeResolver({ kodikApi } as any)
    const eps = await resolver.listEpisodes('kodik', 'uuid')
    expect(eps.length).toBe(3)
    const stream = await resolver.resolveStream('kodik', 'uuid', eps[0], { audio: 'dub', lang: 'ru', provider: 'kodik', server: '', team: 'AniLibria' })
    expect(stream.type).toBe('hls')
    expect(stream.url).toContain('/api/streaming/hls-proxy')
    expect(stream.url).toContain('x.m3u8')
    expect(kodikApi.getStream).toHaveBeenCalledWith('uuid', 1, 7)
  })

  it('uses MAX episodes_count across all kodik translations', async () => {
    const kodikApi = {
      getTranslations: vi.fn().mockResolvedValue({
        data: {
          data: [
            { id: 1, title: 'TeamA', type: 'voice', episodes_count: 12 },
            { id: 2, title: 'TeamB', type: 'sub',   episodes_count: 24 },
            { id: 3, title: 'TeamC', type: 'voice', episodes_count: 6  },
          ],
        },
      }),
      getStream: vi.fn(),
    }
    const resolver = makeResolver({ kodikApi } as any)
    const eps = await resolver.listEpisodes('kodik', 'uuid')
    // Should reflect TeamB's 24, not TeamA's 12
    expect(eps.length).toBe(24)
    expect(eps[23].number).toBe(24)
  })
})
