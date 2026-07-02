import { describe, it, expect } from 'vitest'
import { NAV_DENYLIST, edgeAssetToOriginPath, isOfflinePath } from './swRoutes'

const denied = (path: string) => NAV_DENYLIST.some((re) => re.test(path))

describe('NAV_DENYLIST', () => {
  it('denies API, OG, socket, admin infra, health, sw files, offline ns', () => {
    for (const p of ['/api/anime/x', '/og/home', '/socket.io/?x=1', '/admin/grafana/d/1',
      '/admin/prometheus/graph', '/admin/pgadmin/', '/admin/k8s/', '/health',
      '/sw.js', '/sw-config.json', '/__offline/abc/master.m3u8']) {
      expect(denied(p), p).toBe(true)
    }
  })
  it('allows SPA routes including /admin/feedback (SPA admin UI)', () => {
    for (const p of ['/', '/anime/uuid-1', '/downloads', '/browse', '/admin/feedback']) {
      expect(denied(p), p).toBe(false)
    }
  })
})

describe('edgeAssetToOriginPath', () => {
  const origin = 'https://animeenigma.org'
  it('maps cross-origin /assets/ chunk to same-path origin URL', () => {
    expect(edgeAssetToOriginPath('https://msk-edge.example/assets/chunk-abc.js', origin))
      .toBe('https://animeenigma.org/assets/chunk-abc.js')
  })
  it('returns null for same-origin, non-assets, and garbage', () => {
    expect(edgeAssetToOriginPath('https://animeenigma.org/assets/a.js', origin)).toBeNull()
    expect(edgeAssetToOriginPath('https://msk-edge.example/api/x', origin)).toBeNull()
    expect(edgeAssetToOriginPath('not a url', origin)).toBeNull()
  })
})

describe('isOfflinePath', () => {
  it('matches only the /__offline/ namespace', () => {
    expect(isOfflinePath('/__offline/id1/master.m3u8')).toBe(true)
    expect(isOfflinePath('/offline')).toBe(false)
  })
})
