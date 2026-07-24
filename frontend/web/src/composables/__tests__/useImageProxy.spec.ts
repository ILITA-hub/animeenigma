import { describe, it, expect } from 'vitest'
import { cardPosterUrl } from '../useImageProxy'

describe('cardPosterUrl', () => {
  const shiki = 'https://shikimori.io/uploads/poster/animes/59062/eef2d.jpeg'

  it('returns placeholder for empty input', () => {
    expect(cardPosterUrl(undefined, 128)).toBe('/placeholder.svg')
    expect(cardPosterUrl(null, 128)).toBe('/placeholder.svg')
    expect(cardPosterUrl('', 128)).toBe('/placeholder.svg')
  })

  it('proxies shikimori URLs with the snapped width bucket', () => {
    expect(cardPosterUrl(shiki, 128)).toBe(
      `/api/streaming/image-proxy?url=${encodeURIComponent(shiki)}&w=128`
    )
  })

  it('snaps width UP to the nearest bucket', () => {
    expect(cardPosterUrl(shiki, 129)).toContain('w=256')
    expect(cardPosterUrl(shiki, 300)).toContain('w=384')
  })

  it('clamps widths above the largest bucket to 640', () => {
    expect(cardPosterUrl(shiki, 4096)).toContain('w=640')
  })

  it('proxies MAL CDN URLs', () => {
    const mal = 'https://cdn.myanimelist.net/images/anime/1/2.jpg'
    expect(cardPosterUrl(mal, 384)).toContain('/api/streaming/image-proxy?url=')
  })

  it('routes relative gacha image URLs through the proxy', () => {
    const gacha = '/api/gacha/images/cards/x.png'
    expect(cardPosterUrl(gacha, 128)).toBe(
      `/api/streaming/image-proxy?url=${encodeURIComponent(gacha)}&w=128`
    )
  })

  it('passes non-proxyable URLs through unchanged', () => {
    expect(cardPosterUrl('https://example.com/p.jpg', 128)).toBe('https://example.com/p.jpg')
    expect(cardPosterUrl('/placeholder.svg', 128)).toBe('/placeholder.svg')
  })

  it('passes non-proxyable absolute URLs through', () => {
    expect(cardPosterUrl('https://example.com/a.png', 128)).toBe('https://example.com/a.png')
  })
})
