import { describe, it, expect } from 'vitest'
import { parseRange, buildRangeResponse, handleOfflineRequest } from './offlineServe'

function fakeCachesWith(entries: Record<string, Response>): CacheStorage {
  const cache = {
    async match(req: string | Request) {
      const path = typeof req === 'string' ? req : new URL(req.url).pathname
      return entries[path]?.clone()
    },
  }
  return { async open() { return cache as unknown as Cache } } as unknown as CacheStorage
}

describe('parseRange', () => {
  it('parses closed, open-ended, and rejects invalid', () => {
    expect(parseRange('bytes=0-99', 1000)).toEqual({ start: 0, end: 99 })
    expect(parseRange('bytes=500-', 1000)).toEqual({ start: 500, end: 999 })
    expect(parseRange('bytes=990-2000', 1000)).toEqual({ start: 990, end: 999 })
    expect(parseRange('bytes=1000-', 1000)).toBeNull() // start beyond size
    expect(parseRange('items=0-1', 1000)).toBeNull()
  })
})

describe('buildRangeResponse', () => {
  it('slices a cached body into a 206 with correct headers', async () => {
    const body = new Uint8Array(100).map((_, i) => i)
    const full = new Response(body, { headers: { 'Content-Type': 'video/mp4' } })
    const r = await buildRangeResponse(full, 'bytes=10-19')
    expect(r.status).toBe(206)
    expect(r.headers.get('Content-Range')).toBe('bytes 10-19/100')
    expect(r.headers.get('Content-Length')).toBe('10')
    expect(r.headers.get('Accept-Ranges')).toBe('bytes')
    expect(new Uint8Array(await r.arrayBuffer())[0]).toBe(10)
  })
  it('416 on unsatisfiable range', async () => {
    const full = new Response(new Uint8Array(10))
    expect((await buildRangeResponse(full, 'bytes=50-')).status).toBe(416)
  })
})

describe('handleOfflineRequest', () => {
  const entries = {
    '/__offline/d1/master.m3u8': new Response('#EXTM3U', { headers: { 'Content-Type': 'application/vnd.apple.mpegurl' } }),
    '/__offline/d1/media.mp4': new Response(new Uint8Array(100), { headers: { 'Content-Type': 'video/mp4' } }),
  }
  it('serves cached entries as-is without Range', async () => {
    const r = await handleOfflineRequest(new Request('https://x/__offline/d1/master.m3u8'), fakeCachesWith(entries))
    expect(r.status).toBe(200)
    expect(await r.text()).toBe('#EXTM3U')
  })
  it('serves 206 slices for ranged mp4 requests', async () => {
    const req = new Request('https://x/__offline/d1/media.mp4', { headers: { Range: 'bytes=0-9' } })
    const r = await handleOfflineRequest(req, fakeCachesWith(entries))
    expect(r.status).toBe(206)
    expect(r.headers.get('Content-Range')).toBe('bytes 0-9/100')
  })
  it('404 on cache miss (evicted or bogus id)', async () => {
    const r = await handleOfflineRequest(new Request('https://x/__offline/nope/master.m3u8'), fakeCachesWith(entries))
    expect(r.status).toBe(404)
  })
})
