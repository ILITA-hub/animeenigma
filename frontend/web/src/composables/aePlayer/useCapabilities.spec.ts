import { describe, it, expect } from 'vitest'
import { flattenCapabilities } from './useCapabilities'
import type { CapabilityReport } from '@/types/capabilities'

// Feed defaults so each literal only spells out the fields under test.
const feed = { state: 'active' as const, selectable: true, hacker_only: false, order: 0, group: 'en' as const, audios: ['sub'] as ('sub' | 'dub')[] }

const report: CapabilityReport = {
  anime_id: 'uuid-1',
  families: [
    {
      family: 'others',
      providers: [
        { ...feed, provider: 'gogoanime', display_name: 'Gogoanime', variants: [] },
        { ...feed, provider: 'allanime', display_name: 'AllAnime', variants: [] },
      ],
    },
    {
      family: 'others',
      providers: [
        { ...feed, provider: 'kodik', display_name: 'Kodik', group: 'ru', variants: [] },
      ],
    },
  ],
}

describe('flattenCapabilities', () => {
  it('flattens every family into a provider map keyed by provider id', () => {
    const capMap = flattenCapabilities(report)
    expect(capMap.size).toBe(3)
    expect(capMap.get('gogoanime')?.display_name).toBe('Gogoanime')
    expect(capMap.get('kodik')?.display_name).toBe('Kodik')
  })

  it('degrades to an empty map on null/malformed report', () => {
    expect(flattenCapabilities(null).size).toBe(0)
    expect(flattenCapabilities({ anime_id: 'x' } as unknown as CapabilityReport).size).toBe(0)
  })
})
