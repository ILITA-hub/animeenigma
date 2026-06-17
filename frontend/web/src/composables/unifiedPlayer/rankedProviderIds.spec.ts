import { describe, it, expect } from 'vitest'
import { rankedProviderIds } from './rankedProviderIds'
import type { ProviderRow, ProviderDef } from '@/types/unifiedPlayer'

function row(id: string, state: ProviderRow['state'] = 'active'): ProviderRow {
  return { def: { id } as ProviderDef, state }
}

const CURATED = ['ae', 'allanime', 'gogoanime', 'raw', '18anime']

describe('rankedProviderIds', () => {
  it('puts capability-ranked rows first, in ranked order', () => {
    const rows = [row('allanime'), row('gogoanime'), row('ae'), row('raw')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    expect(out).toEqual(['gogoanime', 'allanime', 'ae', 'raw'])
  })

  it('appends rows absent from the ranking in CURATED order then alpha', () => {
    const rows = [row('raw'), row('ae'), row('zzz')]
    const out = rankedProviderIds(rows, [], CURATED)
    expect(out).toEqual(['ae', 'raw', 'zzz'])
  })

  it('ignores ranked ids that are not present as rows', () => {
    const rows = [row('ae')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    expect(out).toEqual(['ae'])
  })

  it('forces degraded rows to the very end regardless of rank/curated (AUTO-484)', () => {
    const rows = [row('allanime'), row('gogoanime'), row('animefever', 'degraded'), row('ae')]
    // even if the ranking lists animefever first, it must end up last
    const out = rankedProviderIds(rows, ['animefever', 'gogoanime', 'allanime'], CURATED)
    expect(out[out.length - 1]).toBe('animefever')
    expect(out).toEqual(['gogoanime', 'allanime', 'ae', 'animefever'])
  })
})
