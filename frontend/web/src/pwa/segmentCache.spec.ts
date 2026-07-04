import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  SEG_TTL_MS, segmentCacheKey, isScrubRequest, markScrubUrl, handleSegmentRequest,
} from './segmentCache'

const SEG = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/segment_001.ts') + '&exp=111&sig=aaa&sess=s1'
const SEG_RESIGNED = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/segment_001.ts') + '&exp=222&sig=bbb'
const PLAYLIST = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/playlist.m3u8') + '&exp=1&sig=a'
const MP4 = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/movie.mp4') + '&type=mp4'

describe('segmentCacheKey', () => {
  it('keys segments by upstream url only — resigned URL maps to the same key', () => {
    const k = segmentCacheKey(SEG)
    expect(k).toBeTruthy()
    expect(segmentCacheKey(SEG_RESIGNED)).toBe(k)
  })
  it('works on a dedicated stream origin (VITE_HLS_PROXY_BASE)', () => {
    expect(segmentCacheKey(SEG.replace('animeenigma.org', 'stream.animeenigma.org'))).toBe(segmentCacheKey(SEG))
  })
  it('rejects playlists, mp4-progressive, foreign paths, and garbage', () => {
    expect(segmentCacheKey(PLAYLIST)).toBeNull()
    expect(segmentCacheKey(MP4)).toBeNull()
    expect(segmentCacheKey('https://animeenigma.org/api/anime/x')).toBeNull()
    expect(segmentCacheKey('not a url')).toBeNull()
  })
})

describe('markScrubUrl / isScrubRequest', () => {
  it('appends the marker to hls-proxy urls only, idempotently', () => {
    const m = markScrubUrl(SEG)
    expect(isScrubRequest(m)).toBe(true)
    expect(isScrubRequest(SEG)).toBe(false)
    expect(markScrubUrl(m)).toBe(m)
    expect(markScrubUrl('https://x.example/other')).toBe('https://x.example/other')
  })
  it('marker does not change the cache key', () => {
    expect(segmentCacheKey(markScrubUrl(SEG))).toBe(segmentCacheKey(SEG))
  })
})

describe('handleSegmentRequest', () => {
  let store: Map<string, Response>
  const fakeCache = {
    match: vi.fn(async (k: string) => store.get(k)),
    put: vi.fn(async (k: string, r: Response) => void store.set(k, r)),
    delete: vi.fn(async (k: string) => store.delete(k)),
    keys: vi.fn(async () => [...store.keys()].map((k) => new Request(k))),
  }
  const waits: Promise<unknown>[] = []
  const event = { waitUntil: (p: Promise<unknown>) => waits.push(p) } as unknown as FetchEvent

  beforeEach(() => {
    store = new Map()
    waits.length = 0
    vi.stubGlobal('caches', { open: async () => fakeCache })
    vi.stubGlobal('navigator', { storage: { estimate: async () => ({ usage: 0, quota: 50_000_000_000 }) } })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })))
  })
  afterEach(() => vi.unstubAllGlobals())

  it('non-scrub: returns network response and tees the copy via waitUntil', async () => {
    const resp = await handleSegmentRequest(new Request(SEG), event)
    expect(resp.status).toBe(200)
    await Promise.all(waits)
    expect(fakeCache.put).toHaveBeenCalledTimes(1)
  })
  it('scrub hit: served from cache without fetch', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    ;(fetch as ReturnType<typeof vi.fn>).mockClear()
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG_RESIGNED)), event)
    expect(resp.status).toBe(200)
    expect(fetch).not.toHaveBeenCalled()
  })
  it('non-scrub requests are NEVER served from cache, even when the entry exists', async () => {
    fakeCache.put.mockClear() // clear accumulated calls from previous tests
    await handleSegmentRequest(new Request(SEG), event) // warm the cache
    await Promise.all(waits)
    expect(fakeCache.put).toHaveBeenCalledTimes(1)
    ;(fetch as ReturnType<typeof vi.fn>).mockClear()
    const resp = await handleSegmentRequest(new Request(SEG_RESIGNED), event) // same key, no aescrub
    expect(fetch).toHaveBeenCalledTimes(1) // network, not cache
    expect(resp.status).toBe(200)
  })
  it('scrub miss: falls through to network', async () => {
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG)), event)
    expect(resp.status).toBe(200)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('expired entries are treated as misses', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    const key = segmentCacheKey(SEG)!
    const old = store.get(key)!
    const h = new Headers(old.headers)
    h.set('x-ae-cached-at', String(Date.now() - SEG_TTL_MS - 1))
    store.set(key, new Response(await old.arrayBuffer(), { headers: h }))
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG)), event)
    expect(fetch).toHaveBeenCalled()
    expect(resp.status).toBe(200)
  })
  it('cache write failures never affect the returned response', async () => {
    fakeCache.put.mockRejectedValueOnce(new Error('quota'))
    const resp = await handleSegmentRequest(new Request(SEG), event)
    expect(resp.status).toBe(200)
    await Promise.all(waits) // must not reject
  })
  it('ranged requests bypass cache read AND tee', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    fakeCache.put.mockClear()
    ;(fetch as ReturnType<typeof vi.fn>).mockClear()
    const ranged = new Request(markScrubUrl(SEG), { headers: { range: 'bytes=0-100' } })
    await handleSegmentRequest(ranged, event)
    expect(fetch).toHaveBeenCalledTimes(1) // no cache hit despite the entry existing
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
  })
  it('skips writes when storage headroom is low or estimate unavailable', async () => {
    vi.stubGlobal('navigator', { storage: { estimate: async () => ({ usage: 0, quota: 500_000_000 }) } })
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
    vi.stubGlobal('navigator', {})
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
  })
})
