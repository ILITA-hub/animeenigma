import { describe, it, expect, vi } from 'vitest'
import { makeResolver, NotAvailableError } from './useProviderResolver'

/** Parse the query params of a `/api/streaming/hls-proxy?...` URL. */
function proxyParams(url: string): URLSearchParams {
  expect(url.startsWith('/api/streaming/hls-proxy?')).toBe(true)
  return new URLSearchParams(url.split('?')[1])
}

describe('useProviderResolver', () => {
  it('dispatches to the scraper adapter for an EN provider and proxies the stream with its Referer', async () => {
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
              headers: { Referer: 'https://allmanga.to' },
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
    // The raw CDN url must NOT be handed to the <video>/hls.js directly — it has
    // to be wrapped through the backend HLS proxy so the required Referer can be
    // injected (the CDN 403s / hangs without it).
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('http://x/m3u8')
    expect(params.get('referer')).toBe('https://allmanga.to')
    expect(params.get('type')).toBeNull() // hls → no type=mp4 marker
    expect(scraperApi.getEpisodes).toHaveBeenCalledWith('anime-uuid', 'allanime')
  })

  it('forwards the provenance exp/sig of a scraper source to the proxy url', async () => {
    // The catalog signs scraper stream URLs (exp/sig siblings on each source) so
    // the HLS proxy trusts non-allowlisted CDNs (megaplay's streamzone1.site, …)
    // WITHOUT a static allowlist entry. If the resolver drops them, the proxy
    // 502s and only providers on the legacy allowlist (miruro) play.
    const scraperApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: { episodes: [{ id: 'e1', number: 1 }], meta: { provider: 'gogoanime' } } },
      }),
      getServers: vi.fn().mockResolvedValue({
        data: { data: { servers: [{ id: 's1', name: 'Server 1' }] } },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: {
          data: {
            stream: {
              sources: [{ url: 'https://s1.streamzone1.site/master.m3u8', type: 'hls', exp: '1781731463', sig: 'deadbeef' }],
              headers: { Referer: 'https://megaplay.buzz/' },
            },
          },
        },
      }),
    }
    const resolver = makeResolver({ scraperApi } as any)
    const eps = await resolver.listEpisodes('gogoanime', 'anime-uuid')
    const stream = await resolver.resolveStream('gogoanime', 'anime-uuid', eps[0], {
      audio: 'sub',
      lang: 'en',
      provider: 'gogoanime',
      server: 's1',
      team: null,
    })
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('https://s1.streamzone1.site/master.m3u8')
    expect(params.get('exp')).toBe('1781731463')
    expect(params.get('sig')).toBe('deadbeef')
  })

  it('marks scraper MP4 streams with type=mp4 in the proxy url', async () => {
    const scraperApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: { episodes: [{ id: 'e1', number: 1 }] } },
      }),
      getServers: vi.fn().mockResolvedValue({
        data: { data: { servers: [{ id: 'Yt-mp4', name: 'Yt-mp4', type: 'sub' }] } },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: {
          data: {
            stream: {
              sources: [{ url: 'https://tools.fast4speed.rsvp/v/1', type: 'mp4' }],
              headers: { Referer: 'https://allmanga.to' },
            },
          },
        },
      }),
    }
    const resolver = makeResolver({ scraperApi } as any)
    const eps = await resolver.listEpisodes('allanime', 'uuid')
    const stream = await resolver.resolveStream('allanime', 'uuid', eps[0], {
      audio: 'sub', lang: 'en', provider: 'allanime', server: 'Yt-mp4', team: null,
    })
    expect(stream.type).toBe('mp4')
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('https://tools.fast4speed.rsvp/v/1')
    expect(params.get('referer')).toBe('https://allmanga.to')
    expect(params.get('type')).toBe('mp4')
  })

  it('proxies the raw (AllAnime JP) stream with the allmanga.to Referer', async () => {
    const rawApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: { episodes: [{ id: 'r1', number: 1, title: 'Ep 1' }], available: true, source: 'allanime' } },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: { url: 'https://tools.fast4speed.rsvp/raw/1', type: 'mp4' } },
      }),
    }
    const resolver = makeResolver({ rawApi } as any)
    const eps = await resolver.listEpisodes('raw', 'uuid')
    const stream = await resolver.resolveStream('raw', 'uuid', eps[0], {
      audio: 'sub', lang: 'ja', provider: 'raw', server: '', team: null,
    })
    expect(stream.type).toBe('mp4')
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('https://tools.fast4speed.rsvp/raw/1')
    expect(params.get('referer')).toBe('https://allmanga.to/')
    expect(params.get('type')).toBe('mp4')
    expect(rawApi.getStream).toHaveBeenCalledWith('uuid', 1)
  })

  it('throws a typed error for a disabled/unwired provider', async () => {
    const resolver = makeResolver({} as any)
    await expect(resolver.listEpisodes('animelib', 'x')).rejects.toThrow(/not available/i)
  })

  it('wires the first-party ae provider: library episodes + signed minio stream', async () => {
    const aeApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: { episodes: [{ id: '1', number: 1, title: '' }, { id: '2', number: 2, title: '' }], available: true, source: 'library' } },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: {
          url: 'http://minio:9000/raw-library/54974/1/playlist.m3u8',
          type: 'hls', source: 'library', exp: '1799999999', sig: 'deadbeef',
        } },
      }),
    }
    const resolver = makeResolver({ aeApi } as any)
    const eps = await resolver.listEpisodes('ae', 'uuid')
    expect(aeApi.getEpisodes).toHaveBeenCalledWith('uuid')
    expect(eps.length).toBe(2)
    expect(eps[0].number).toBe(1)

    const stream = await resolver.resolveStream('ae', 'uuid', eps[0], {
      audio: 'sub', lang: 'ja', provider: 'ae', server: '', team: null,
    })
    expect(aeApi.getStream).toHaveBeenCalledWith('uuid', 1)
    expect(stream.type).toBe('hls')
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('http://minio:9000/raw-library/54974/1/playlist.m3u8')
    // The proxy signature MUST be forwarded — minio is not allowlisted.
    expect(params.get('exp')).toBe('1799999999')
    expect(params.get('sig')).toBe('deadbeef')
    // MinIO needs no Referer.
    expect(params.get('referer')).toBeNull()
  })

  it('ae: surfaces a typed error when the episode has no local copy', async () => {
    const aeApi = {
      getEpisodes: vi.fn(),
      getStream: vi.fn().mockResolvedValue({ data: { data: { url: '' } } }),
    }
    const resolver = makeResolver({ aeApi } as any)
    await expect(
      resolver.resolveStream('ae', 'uuid', { key: 5, label: 5, number: 5 }, {
        audio: 'sub', lang: 'ja', provider: 'ae', server: '', team: null,
      }),
    ).rejects.toThrow(/local copy/i)
  })

  it('routes 18anime to the anime18 adapter (NOT the scraper)', async () => {
    const scraperApi = { getEpisodes: vi.fn(), getServers: vi.fn(), getStream: vi.fn() }
    const anime18Api = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: [{ slug: 'ep-1', number: 1 }] },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: { url: 'http://x/h.m3u8', referer: 'https://18anime.ref', is_hls: true, quality: '720p' } },
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
    // Must be proxied with the source's Referer (mp4upload etc. require it).
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('http://x/h.m3u8')
    expect(params.get('referer')).toBe('https://18anime.ref')
    expect(stream.type).toBe('hls')
  })

  it('wires kodik via translations + proxy-wrapped stream', async () => {
    localStorage.clear()
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
    // No saved preference -> requests the max-quality sentinel (Kodik's own
    // default is 360p; the backend clamps 2160 to the highest available).
    expect(kodikApi.getStream).toHaveBeenCalledWith('uuid', 1, 7, 2160)
  })

  it('kodik: requests the saved quality preference and exposes the per-URL ladder', async () => {
    localStorage.setItem('kodik_adfree_quality', '480')
    const kodikApi = {
      getTranslations: vi.fn().mockResolvedValue({ data: { data: [{ id: 7, title: 'AniLibria', type: 'voice', episodes_count: 3 }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { data: {
        stream_url: 'http://cdn/480.m3u8', referer: 'https://kodik',
        quality: 480, qualities: [360, 480, 720],
      } } }),
    }
    const resolver = makeResolver({ kodikApi } as any)
    const eps = await resolver.listEpisodes('kodik', 'uuid')
    const stream = await resolver.resolveStream('kodik', 'uuid', eps[0], { audio: 'dub', lang: 'ru', provider: 'kodik', server: '', team: null })
    localStorage.clear()

    expect(kodikApi.getStream).toHaveBeenCalledWith('uuid', 1, 7, 480)
    // Per-URL ladder: numeric values, sorted descending, served quality labeled.
    expect(stream.qualities).toEqual([
      { label: '720p', value: 720 },
      { label: '480p', value: 480 },
      { label: '360p', value: 360 },
    ])
    expect(stream.qualityLabel).toBe('480p')
  })

  it('kodik: single-quality stream exposes no ladder', async () => {
    localStorage.clear()
    const kodikApi = {
      getTranslations: vi.fn().mockResolvedValue({ data: { data: [{ id: 7, title: 'AniLibria', type: 'voice', episodes_count: 1 }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { data: {
        stream_url: 'http://cdn/720.m3u8', referer: 'https://kodik',
        quality: 720, qualities: [720],
      } } }),
    }
    const resolver = makeResolver({ kodikApi } as any)
    const eps = await resolver.listEpisodes('kodik', 'uuid')
    const stream = await resolver.resolveStream('kodik', 'uuid', eps[0], { audio: 'dub', lang: 'ru', provider: 'kodik', server: '', team: null })

    expect(stream.qualities).toBeUndefined()
    expect(stream.qualityLabel).toBe('720p')
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

  it('routes hanime to the hanime adapter (slug-keyed, NOT the scraper)', async () => {
    const scraperApi = { getEpisodes: vi.fn(), getServers: vi.fn(), getStream: vi.fn() }
    const hanimeApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: [{ name: 'Episode 1', slug: 'show-1' }, { name: 'Episode 2', slug: 'show-2' }] },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: { sources: [
          { url: 'http://cdn/480.m3u8', height: '480', width: 854, size_mb: 100 },
          { url: 'http://cdn/1080.m3u8', height: '1080', width: 1920, size_mb: 500 },
        ] } },
      }),
    }
    const resolver = makeResolver({ scraperApi, hanimeApi } as any)
    const eps = await resolver.listEpisodes('hanime', 'uuid')
    expect(hanimeApi.getEpisodes).toHaveBeenCalledWith('uuid')
    expect(scraperApi.getEpisodes).not.toHaveBeenCalled()
    expect(eps.length).toBe(2)
    expect(eps[0].key).toBe('show-1') // slug-keyed
    expect(eps[0].number).toBe(1)     // ordinal derived from index
    const stream = await resolver.resolveStream('hanime', 'uuid', eps[0], {
      audio: 'dub', lang: 'ru', provider: 'hanime', server: '', team: null,
    })
    expect(hanimeApi.getStream).toHaveBeenCalledWith('uuid', 'show-1')
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('http://cdn/1080.m3u8') // highest-res source
    expect(stream.type).toBe('hls')
  })

  it('throws NotAvailableError when hanime returns no sources', async () => {
    const hanimeApi = {
      getEpisodes: vi.fn().mockResolvedValue({ data: { data: [{ name: 'E1', slug: 's1' }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { data: { sources: [] } } }),
    }
    const resolver = makeResolver({ hanimeApi } as any)
    const eps = await resolver.listEpisodes('hanime', 'uuid')
    await expect(
      resolver.resolveStream('hanime', 'uuid', eps[0], { audio: 'dub', lang: 'ru', provider: 'hanime', server: '', team: null }),
    ).rejects.toThrow(/no stream URL/)
  })

  it('throws NotAvailableError for hanime when the hanimeApi dep is missing', async () => {
    const resolver = makeResolver({} as any)
    await expect(resolver.listEpisodes('hanime', 'uuid')).rejects.toThrow(NotAvailableError)
  })
})

describe('ProviderResolver.listTeams', () => {
  // Mixed sub/dub roster: type 'voice' = dub, anything else = sub.
  const kodikApi = {
    getTranslations: async () => ({ data: { data: [
      { id: 1, title: 'AniLibria',   type: 'voice',    episodes_count: 12 },
      { id: 2, title: 'AniDUB',      type: 'voice',    episodes_count: 12 },
      { id: 3, title: 'AniLibria',   type: 'voice',    episodes_count: 8  }, // dup title
      { id: 4, title: 'SovetRomantica', type: 'subtitles', episodes_count: 12 },
      { id: 5, title: 'Crunchyroll', type: 'subtitles', episodes_count: 12 },
    ] } }),
    getStream: async () => ({ data: { data: {} } }),
  } as never

  it('returns ONLY dub teams when audio is dub (unique, first-seen order)', async () => {
    const resolver = makeResolver({ kodikApi })
    expect(await resolver.listTeams('kodik', 'anime-1', 'dub')).toEqual(['AniLibria', 'AniDUB'])
  })

  it('returns ONLY sub teams when audio is sub — no DUB teams leak in', async () => {
    const resolver = makeResolver({ kodikApi })
    expect(await resolver.listTeams('kodik', 'anime-1', 'sub')).toEqual(['SovetRomantica', 'Crunchyroll'])
  })

  it('returns [] for providers without team support', async () => {
    const resolver = makeResolver({})
    expect(await resolver.listTeams('allanime', 'anime-1', 'sub')).toEqual([])
  })
})
