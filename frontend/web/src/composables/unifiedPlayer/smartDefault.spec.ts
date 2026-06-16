import { describe, it, expect, vi, beforeEach } from 'vitest'
import { pickSmartDefault } from './smartDefault'
import type { ProviderRow } from '@/types/unifiedPlayer'

const row = (id: string, state: ProviderRow['state']): ProviderRow =>
  ({ def: { id, name: id, hue: '#000', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state })

const CURATED = ['ae', 'allanime', 'gogoanime', 'kodik']

describe('pickSmartDefault', () => {
  const alwaysAvailable = vi.fn(async () => true)

  beforeEach(() => { alwaysAvailable.mockClear() })

  it('picks the first active provider in curated order', async () => {
    const rows = [row('gogoanime', 'active'), row('allanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('allanime') // allanime precedes gogoanime in CURATED
  })

  it('skips non-active rows', async () => {
    const rows = [row('allanime', 'down'), row('gogoanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('gogoanime')
  })

  it('excludes an availability-gated provider when unavailable, picks next', async () => {
    const rows = [row('ae', 'active'), row('allanime', 'active')]
    const isAvailable = vi.fn(async () => false)
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(['ae']), isAvailable }))
      .toBe('allanime')
    expect(isAvailable).toHaveBeenCalledWith('ae')
  })

  it('includes an availability-gated provider when available', async () => {
    const rows = [row('ae', 'active'), row('allanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(['ae']), isAvailable: vi.fn(async () => true) }))
      .toBe('ae')
  })

  it('returns null when no rows are active', async () => {
    const rows = [row('ae', 'down'), row('allanime', 'irrelevant')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBeNull()
  })

  it('falls back to first active not in the curated list', async () => {
    const rows = [row('exotic', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('exotic')
  })
})
