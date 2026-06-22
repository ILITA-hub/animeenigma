import { describe, it, expect, vi } from 'vitest'
vi.mock('@/utils/streaming', () => ({ hlsProxyUrl: (q: string) => `/api/streaming/hls-proxy?${q}` }))
import { buildSubtitleProxyUrl, detectSubFormat, langFromTrack } from './subtitleProxy'

describe('subtitleProxy', () => {
  it('builds a signed proxy url', () => {
    const u = buildSubtitleProxyUrl('https://cdn/x.vtt', 'E', 'S')
    expect(u).toContain('url=https%3A%2F%2Fcdn%2Fx.vtt')
    expect(u).toContain('exp=E')
    expect(u).toContain('sig=S')
  })
  it('omits exp/sig when absent', () => {
    expect(buildSubtitleProxyUrl('https://cdn/x.vtt')).not.toContain('exp=')
  })
  it('detects format from explicit value then extension', () => {
    expect(detectSubFormat('ASS', 'x')).toBe('ass')
    expect(detectSubFormat(undefined, 'https://c/a.srt?token=1')).toBe('srt')
    expect(detectSubFormat(undefined, 'https://c/a.bin')).toBeNull()
  })
  it('infers lang from label keywords, else ja default', () => {
    expect(langFromTrack('English', 'x')).toBe('en')
    expect(langFromTrack('Русский', 'x')).toBe('ru')
    expect(langFromTrack(undefined, 'https://c/jpn.vtt')).toBe('ja')
    expect(langFromTrack('日本語', 'x')).toBe('ja')
  })
})
