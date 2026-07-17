import { describe, it, expect } from 'vitest'
import { deriveCapLabels } from './capLabels'
import { streamBadgeText } from './streamBadgeText'
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

// i18n stub — English key leaves, uppercased by CSS in the real chip.
const t = (k: string) =>
  (({ 'player.sub': 'Sub', 'player.dub': 'Dub', 'player.sources.subBurnedIn': 'burned-in', 'player.sources.subSelectable': 'selectable' }) as Record<string, string>)[k] ?? k

describe('deriveCapLabels', () => {
  it('collects the max quality across variants', () => {
    const l = deriveCapLabels(cap([
      { category: 'dub', sub_delivery: 'none', qualities: ['720p'], quality_source: 'trait', source: 'trait' },
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]))
    expect(l?.quality).toBe('1080p')
  })

  it('returns null for missing/empty variants', () => {
    expect(deriveCapLabels(undefined)).toBeNull()
    expect(deriveCapLabels(cap([]))).toBeNull()
  })

  const variants: ProviderCap['variants'] = [
    { category: 'sub', sub_delivery: 'soft', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
  ]

  it('unverified non-firstparty: marker set, NO badges (nothing is claimed)', () => {
    const l = deriveCapLabels(cap(variants))
    expect(l?.unverified).toBe(true)
    expect(l?.badges).toEqual([])

    const l2 = deriveCapLabels(cap(variants), v({}))
    expect(l2?.unverified).toBe(true)
    expect(l2?.badges).toEqual([])
  })

  it('firstparty caps are never marked unverified and badge from their own variants', () => {
    const l = deriveCapLabels(cap(variants, 'firstparty'), null)
    expect(l?.unverified).toBe(false)
    expect(l?.badges).toEqual([{ kind: 'sub', langs: [], burnedIn: false, soft: true }])
  })

  // The owner's canonical kodik line: "SUB BURNED-IN RU • DUB RU" — exactly
  // ONE badge per stream kind, langs folded in, never parallel per-fact chips.
  it('verified: one consolidated badge per kind with langs folded in', () => {
    const l = deriveCapLabels(cap(variants), v({ status: 'verified', raw: true, dub_langs: ['ru'], hardsub_langs: ['ru'] }))
    expect(l?.unverified).toBe(false)
    expect(l?.badges).toEqual([
      { kind: 'sub', langs: ['ru'], burnedIn: true, soft: false },
      { kind: 'dub', langs: ['ru'], burnedIn: false, soft: false },
    ])
    expect(l!.badges.map((b) => streamBadgeText(b, t))).toEqual(['Sub burned-in RU', 'Dub RU'])
  })

  it('partial verify keeps a plain sub badge (unverified units stay RAW-assumed) and adds proven dub', () => {
    const l = deriveCapLabels(cap(variants), v({ status: 'partial', dub_langs: ['ru'] }))
    expect(l?.unverified).toBe(false)
    expect(l?.badges).toEqual([
      { kind: 'sub', langs: [], burnedIn: false, soft: true },
      { kind: 'dub', langs: ['ru'], burnedIn: false, soft: false },
    ])
  })

  it('verified raw with no burned subs keeps hard delivery from the variant claim', () => {
    const hardVariants: ProviderCap['variants'] = [
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]
    const l = deriveCapLabels(cap(hardVariants), v({ status: 'verified', raw: true }))
    expect(l?.badges).toEqual([{ kind: 'sub', langs: [], burnedIn: true, soft: false }])
    expect(streamBadgeText(l!.badges[0], t)).toBe('Sub · burned-in')
  })

  // firstparty (ae) CAN carry a verify row — synthesized from library truth —
  // but must still trust its own variants (a dub-only verdict must not drop
  // the sub variant ae actually serves).
  it('firstparty caps keep variant-derived badges even WITH a verify row', () => {
    const subDubVariants: ProviderCap['variants'] = [
      { category: 'sub', sub_delivery: 'soft', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
      { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]
    const l = deriveCapLabels(
      cap(subDubVariants, 'firstparty'),
      v({ status: 'verified', raw: false, dub_langs: ['en'] }),
    )
    expect(l?.unverified).toBe(false)
    expect(l?.badges.map((b) => b.kind)).toEqual(['sub', 'dub'])
    expect(l?.badges[1].langs).toEqual([])
  })

  it('multi-lang dub badge joins langs: "Dub EN/RU"', () => {
    const l = deriveCapLabels(cap(variants), v({ status: 'verified', raw: false, dub_langs: ['en', 'ru'] }))
    expect(streamBadgeText(l!.badges[0], t)).toBe('Dub EN/RU')
  })
})
