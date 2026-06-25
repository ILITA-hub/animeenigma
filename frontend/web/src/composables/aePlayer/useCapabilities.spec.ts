import { describe, it, expect } from 'vitest'
import { flattenCapabilities } from './useCapabilities'
import type { CapabilityReport } from '@/types/capabilities'

// Feed defaults so each literal only spells out the fields under test (rank).
const feed = { state: 'active' as const, selectable: true, hacker_only: false, order: 0, group: 'en' as const, audios: ['sub'] as ('sub' | 'dub' | 'raw')[] }

const report: CapabilityReport = {
  anime_id: 'uuid-1',
  families: [
    {
      family: 'ourenglish',
      providers: [
        { ...feed, provider: 'gogoanime', display_name: 'Gogoanime', enabled: true, health: 'up', playable: true, rank: 120, variants: [] },
        { ...feed, provider: 'allanime', display_name: 'AllAnime', enabled: true, health: 'up', rank: 75, variants: [] },
      ],
    },
    {
      family: 'kodik',
      providers: [
        { ...feed, provider: 'kodik', display_name: 'Kodik', enabled: true, health: 'unknown', rank: 0, group: 'ru', variants: [] },
      ],
    },
  ],
}

describe('flattenCapabilities', () => {
  it('flattens every family into a provider map', () => {
    const { capMap } = flattenCapabilities(report)
    expect(capMap.size).toBe(3)
    expect(capMap.get('gogoanime')?.rank).toBe(120)
    expect(capMap.get('kodik')?.display_name).toBe('Kodik')
  })

  it('ranks ids by rank desc with name tiebreak', () => {
    const { rankedIds } = flattenCapabilities(report)
    expect(rankedIds).toEqual(['gogoanime', 'allanime', 'kodik'])
  })

  it('degrades to empty on null/malformed report', () => {
    expect(flattenCapabilities(null).capMap.size).toBe(0)
    expect(flattenCapabilities(null).rankedIds).toEqual([])
    expect(flattenCapabilities({ anime_id: 'x' } as unknown as CapabilityReport).rankedIds).toEqual([])
  })
})
