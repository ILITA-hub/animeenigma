import { describe, it, expect } from 'vitest'
import { rankedProviderIds } from './rankedProviderIds'
import type { ProviderRow, ProviderDef } from '@/types/unifiedPlayer'

function row(
  id: string,
  state: ProviderRow['state'] = 'active',
  group = 'en',
): ProviderRow {
  return { def: { id, group } as ProviderDef, state }
}

const CURATED = ['ae', 'allanime', 'gogoanime', 'raw', '18anime']

describe('rankedProviderIds', () => {
  it('leads with the first-party row, then capability-ranked rows in ranked order', () => {
    const rows = [row('allanime'), row('gogoanime'), row('ae', 'active', 'first-party'), row('raw', 'active', 'raw')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    // 'ae' (first-party) leads — first-party content is preferred over scraped
    // sources; the async availability probe drops it when the title isn't on-prem.
    expect(out).toEqual(['ae', 'gogoanime', 'allanime', 'raw'])
  })

  it('puts capability-ranked rows first when no first-party row is present', () => {
    const rows = [row('allanime'), row('gogoanime'), row('raw', 'active', 'raw')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    expect(out).toEqual(['gogoanime', 'allanime', 'raw'])
  })

  it('appends rows absent from the ranking in CURATED order then alpha', () => {
    const rows = [row('raw', 'active', 'raw'), row('ae', 'active', 'first-party'), row('zzz')]
    const out = rankedProviderIds(rows, [], CURATED)
    expect(out).toEqual(['ae', 'raw', 'zzz'])
  })

  it('ignores ranked ids that are not present as rows', () => {
    const rows = [row('ae', 'active', 'first-party')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    expect(out).toEqual(['ae'])
  })
})
