import { describe, it, expect } from 'vitest'
import { deriveCapLabels } from './capLabels'
import type { ProviderCap } from '@/types/capabilities'

function cap(variants: ProviderCap['variants']): ProviderCap {
  return {
    provider: 'x', display_name: 'X',
    state: 'active', selectable: true, hacker_only: false, order: 50,
    group: 'en', audios: ['sub'],
    enabled: true, health: 'up', rank: 1, variants,
  }
}

describe('deriveCapLabels', () => {
  it('collects categories in sub,dub,raw order and the max quality', () => {
    const l = deriveCapLabels(cap([
      { category: 'dub', sub_delivery: 'none', qualities: ['720p'], quality_source: 'trait', source: 'trait' },
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]))
    expect(l?.categories).toEqual(['sub', 'dub'])
    expect(l?.quality).toBe('1080p')
    expect(l?.subDelivery).toBe('hard')
  })

  it('reports soft sub delivery and omits unknown quality', () => {
    const l = deriveCapLabels(cap([
      { category: 'sub', sub_delivery: 'soft', quality_source: 'unknown', source: 'discovered' },
    ]))
    expect(l?.subDelivery).toBe('soft')
    expect(l?.quality).toBeNull()
  })

  it('returns null for missing/empty variants', () => {
    expect(deriveCapLabels(undefined)).toBeNull()
    expect(deriveCapLabels(cap([]))).toBeNull()
  })
})
