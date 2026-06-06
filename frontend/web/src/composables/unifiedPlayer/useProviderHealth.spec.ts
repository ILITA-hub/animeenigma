import { describe, it, expect } from 'vitest'
import { computeProviderRows } from './useProviderHealth'
import type { ScraperProviderHealth } from '@/types/unifiedPlayer'

const health = (over: Partial<ScraperProviderHealth> & { name: string }): ScraperProviderHealth =>
  ({ enabled: true, up: true, ...over })

describe('computeProviderRows', () => {
  it('marks a healthy, relevant scraper provider active', () => {
    const rows = computeProviderRows(
      [health({ name: 'allanime' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === 'allanime')!.state).toBe('active')
  })

  it('marks a registry-disabled scraper provider disabled with its reason', () => {
    const rows = computeProviderRows(
      [health({ name: 'animepahe', enabled: false, reason: 'Cloudflare challenge', description: 'sidecar 0% solve' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    const r = rows.find(r => r.def.id === 'animepahe')!
    expect(r.state).toBe('disabled')
    expect(r.reason).toContain('Cloudflare')
  })

  it('marks an up=false scraper provider down', () => {
    const rows = computeProviderRows(
      [health({ name: 'gogoanime', up: false })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === 'gogoanime')!.state).toBe('down')
  })

  it('marks a non-scraper hard-disabled provider disabled (animelib)', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'ru', content: 'common' })
    expect(rows.find(r => r.def.id === 'animelib')!.state).toBe('disabled')
  })

  it('marks AnimeEnigma wip', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.find(r => r.def.id === 'ae')!.state).toBe('wip')
  })

  it('marks 18anime irrelevant on a common title', () => {
    const rows = computeProviderRows(
      [health({ name: '18anime' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === '18anime')!.state).toBe('irrelevant')
  })

  it('marks a provider irrelevant when audio/lang mismatch (raw on sub/en)', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.find(r => r.def.id === 'raw')!.state).toBe('irrelevant')
  })
})
