import { describe, it, expect, vi, beforeEach } from 'vitest'
import { hlsProxyUrl } from './streaming'

const { currentBase } = vi.hoisted(() => ({ currentBase: vi.fn(() => '') }))

vi.mock('@/utils/protocolLadder', () => ({
  ladder: { currentBase },
}))

describe('hlsProxyUrl()', () => {
  beforeEach(() => {
    currentBase.mockReset()
  })

  it('defaults to a same-origin relative path when the ladder base is empty', () => {
    currentBase.mockReturnValue('')
    expect(hlsProxyUrl('url=https%3A%2F%2Fx.test%2Fa.m3u8')).toBe(
      '/api/streaming/hls-proxy?url=https%3A%2F%2Fx.test%2Fa.m3u8',
    )
  })

  it('prepends the ladder-provided base (no trailing slash) for an absolute subdomain', () => {
    currentBase.mockReturnValue('https://stream.animeenigma.ru')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('strips a trailing slash on the ladder-provided base', () => {
    currentBase.mockReturnValue('https://stream.animeenigma.ru/')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('accepts an empty query string', () => {
    currentBase.mockReturnValue('')
    expect(hlsProxyUrl('')).toBe('/api/streaming/hls-proxy?')
  })

  it('roots the URL at whatever tier base the ladder currently reports', () => {
    currentBase.mockReturnValue('https://stream2.test')
    expect(hlsProxyUrl('url=abc')).toBe('https://stream2.test/api/streaming/hls-proxy?url=abc')
  })
})
