import { describe, it, expect, afterEach, vi } from 'vitest'
import { hlsProxyUrl } from './streaming'

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('hlsProxyUrl()', () => {
  it('defaults to a same-origin relative path when no base is configured', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', '')
    expect(hlsProxyUrl('url=https%3A%2F%2Fx.test%2Fa.m3u8')).toBe(
      '/api/streaming/hls-proxy?url=https%3A%2F%2Fx.test%2Fa.m3u8',
    )
  })

  it('prepends the configured base (no trailing slash) for an absolute subdomain', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', 'https://stream.animeenigma.ru')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('strips a trailing slash on the configured base', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', 'https://stream.animeenigma.ru/')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('accepts an empty query string', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', '')
    expect(hlsProxyUrl('')).toBe('/api/streaming/hls-proxy?')
  })
})
