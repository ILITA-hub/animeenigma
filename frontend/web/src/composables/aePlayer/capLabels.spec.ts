import { describe, it, expect } from 'vitest'
import { deriveCapLabels } from './capLabels'
import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'

function cap(variants: ProviderCap['variants'], group: ProviderCap['group'] = 'en'): ProviderCap {
  return {
    provider: 'x', display_name: 'X',
    state: 'active', selectable: true, hacker_only: false, order: 50,
    group, audios: ['sub'], variants,
  }
}

const v = (p: Partial<ProviderVerify>): ProviderVerify => ({
  status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [], ...p,
})

describe('deriveCapLabels', () => {
  it('collects categories in sub,dub order and the max quality', () => {
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

  const variants: ProviderCap['variants'] = [
    { category: 'sub', sub_delivery: 'soft', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
  ]

  it('marks a non-firstparty cap unverified when there is no verify data, keeping variant-derived categories', () => {
    const l = deriveCapLabels(cap(variants))
    expect(l?.unverified).toBe(true)
    expect(l?.verifiedDub).toEqual([])
    expect(l?.verifiedHardsub).toEqual([])
    expect(l?.categories).toEqual(['sub'])

    const l2 = deriveCapLabels(cap(variants), v({}))
    expect(l2?.unverified).toBe(true)
  })

  it('firstparty caps are never marked unverified, even with no verify row', () => {
    const l = deriveCapLabels(cap(variants, 'firstparty'), null)
    expect(l?.unverified).toBe(false)
  })

  it('derives categories/badges from proven verdicts once the probe confirms them', () => {
    const l = deriveCapLabels(cap(variants), v({ status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: ['ja'] }))
    expect(l?.unverified).toBe(false)
    expect(l?.categories).toEqual(['sub', 'dub'])
    expect(l?.verifiedDub).toEqual(['en'])
    expect(l?.verifiedHardsub).toEqual(['ja'])
  })

  it('partial verify keeps sub (unverified units stay RAW-assumed) and adds dub when a lang is proven', () => {
    const l = deriveCapLabels(cap(variants), v({ status: 'partial', dub_langs: ['ru'] }))
    expect(l?.unverified).toBe(false)
    expect(l?.categories).toEqual(['sub', 'dub'])
    expect(l?.verifiedDub).toEqual(['ru'])
  })

  // firstparty (ae) CAN carry a verify row — the backend worker synthesizes
  // it from library truth rather than probing — but must still trust its own
  // variants everywhere, per the same exemption effectiveAudios/isUnverified
  // apply. Without the guard, a dub-only verdict here would wrongly collapse
  // categories to ['dub'], dropping the sub variant ae actually serves.
  it('firstparty caps keep variant-derived categories even WITH a verify row (never overridden)', () => {
    const subDubVariants: ProviderCap['variants'] = [
      { category: 'sub', sub_delivery: 'soft', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
      { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]
    const l = deriveCapLabels(
      cap(subDubVariants, 'firstparty'),
      v({ status: 'verified', raw: false, dub_langs: ['en'] }),
    )
    expect(l?.unverified).toBe(false)
    expect(l?.categories).toEqual(['sub', 'dub'])
  })
})
