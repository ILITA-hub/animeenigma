import { describe, it, expect } from 'vitest'
import { selectVariant, rewriteMediaPlaylist, isVod } from './playlistRewrite'

const MASTER = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
360/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720
720/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080
1080/index.m3u8
`

const MEDIA = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="https://cdn.example/key.bin",IV=0x1234
#EXT-X-MAP:URI="init.mp4"
#EXTINF:9.6,
seg-000.ts
#EXTINF:9.6,
https://other-cdn.example/seg-001.ts
#EXT-X-ENDLIST
`

describe('selectVariant', () => {
  it('picks the largest variant not exceeding target height', () => {
    expect(selectVariant(MASTER, 720)?.uri).toBe('720/index.m3u8')
    expect(selectVariant(MASTER, 900)?.uri).toBe('720/index.m3u8')
  })
  it('falls back to the smallest variant when all exceed target', () => {
    expect(selectVariant(MASTER, 200)?.uri).toBe('360/index.m3u8')
  })
  it('returns null for a media playlist (no STREAM-INF)', () => {
    expect(selectVariant(MEDIA, 720)).toBeNull()
  })
})

describe('rewriteMediaPlaylist', () => {
  const base = 'https://proxy.example/hls/ep1/index.m3u8'
  it('maps every URI (segments, KEY, MAP) to /__offline paths and resolves relatives', () => {
    const { body, resources } = rewriteMediaPlaylist(MEDIA, base, 'dl1')
    expect(body).toContain('URI="/__offline/dl1/k/0"')
    expect(body).toContain('#EXT-X-MAP:URI="/__offline/dl1/m/0"')
    expect(body).toContain('/__offline/dl1/r/0')
    expect(body).toContain('/__offline/dl1/r/1')
    expect(body).not.toContain('seg-000.ts')
    const urls = Object.fromEntries(resources.map((r) => [r.path, r.url]))
    expect(urls['/__offline/dl1/r/0']).toBe('https://proxy.example/hls/ep1/seg-000.ts')
    expect(urls['/__offline/dl1/r/1']).toBe('https://other-cdn.example/seg-001.ts')
    expect(urls['/__offline/dl1/k/0']).toBe('https://cdn.example/key.bin')
    expect(urls['/__offline/dl1/m/0']).toBe('https://proxy.example/hls/ep1/init.mp4')
  })
  it('keeps non-URI tags byte-identical', () => {
    const { body } = rewriteMediaPlaylist(MEDIA, base, 'dl1')
    expect(body).toContain('#EXT-X-TARGETDURATION:10')
    expect(body).toContain('IV=0x1234')
    expect(body).toContain('#EXT-X-ENDLIST')
  })
})

describe('isVod', () => {
  it('true only with ENDLIST', () => {
    expect(isVod(MEDIA)).toBe(true)
    expect(isVod(MASTER)).toBe(false)
  })
})
