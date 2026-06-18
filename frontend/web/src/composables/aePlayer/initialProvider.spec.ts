import { describe, expect, it } from 'vitest'
import type { ProviderRow } from '@/types/aePlayer'
import { pickInitialProvider } from './initialProvider'

const row = (id: string, state: ProviderRow['state']): ProviderRow =>
  ({ def: { id, name: id, hue: '#000', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state })

const rows: ProviderRow[] = [
  row('kodik', 'active'),
  row('gogoanime', 'active'),
  row('animelib', 'inactive'),
]

describe('pickInitialProvider', () => {
  it('returns the id when it names an active row', () => {
    expect(pickInitialProvider('kodik', rows)).toBe('kodik')
  })

  it('returns null for an inactive provider (falls back to smart default)', () => {
    expect(pickInitialProvider('animelib', rows)).toBeNull()
  })

  it('returns null for a coarse/unknown value like "english"', () => {
    expect(pickInitialProvider('english', rows)).toBeNull()
  })

  it('returns null when no initial provider is given', () => {
    expect(pickInitialProvider(undefined, rows)).toBeNull()
    expect(pickInitialProvider('', rows)).toBeNull()
    expect(pickInitialProvider(null, rows)).toBeNull()
  })
})
