import { describe, it, expect } from 'vitest'
import { pickSmartDefault, pickRawBiased, pickSelectableFallback } from './smartDefault'
import type { ProviderRow } from '@/types/aePlayer'

const row = (
  id: string,
  over: Partial<ProviderRow> = {},
): ProviderRow => ({
  id, label: id, group: 'en', state: 'active', selectable: true,
  hackerOnly: false, order: 50, audios: ['sub'], ...over,
})

describe('pickSmartDefault', () => {
  it('picks the highest-order active+selectable row', () => {
    const rows = [row('gogoanime', { order: 85 }), row('allanime', { order: 90 })]
    expect(pickSmartDefault(rows)?.id).toBe('allanime')
  })

  it('skips no_content ae, picks highest-order active', () => {
    const rows: ProviderRow[] = [
      row('ae', { group: 'firstparty', state: 'no_content', selectable: false, order: 100 }),
      row('gogoanime', { state: 'active', selectable: true, order: 85 }),
    ]
    expect(pickSmartDefault(rows)?.id).toBe('gogoanime')
  })

  it('skips degraded/recovering (not selectable outside hacker mode)', () => {
    const rows = [
      row('animefever', { state: 'degraded', selectable: false, order: 95 }),
      row('gogoanime', { state: 'active', selectable: true, order: 60 }),
    ]
    expect(pickSmartDefault(rows)?.id).toBe('gogoanime')
  })

  it('returns null when nothing is active', () => {
    const rows = [
      row('ae', { state: 'no_content', selectable: false }),
      row('animefever', { state: 'degraded', selectable: false }),
    ]
    expect(pickSmartDefault(rows)).toBeNull()
  })

  it('never picks an active-but-unselectable row', () => {
    const rows = [row('weird', { state: 'active', selectable: false })]
    expect(pickSmartDefault(rows)).toBeNull()
  })
})

describe('pickRawBiased', () => {
  it('prefers the best active row in the requested language group', () => {
    const rows = [
      row('gogoanime', { group: 'en', order: 90 }),
      row('kodik', { group: 'ru', order: 80 }),
    ]
    expect(pickRawBiased(rows, 'ru')?.id).toBe('kodik')
    expect(pickRawBiased(rows, 'en')?.id).toBe('gogoanime')
  })

  it('falls back to the global best when no active row serves the language', () => {
    const rows = [
      row('gogoanime', { group: 'en', order: 90 }),
      row('kodik', { group: 'ru', order: 80 }),
    ]
    expect(pickRawBiased(rows, 'ja')?.id).toBe('gogoanime')
  })
})

describe('pickSelectableFallback', () => {
  it('returns the top-ranked selectable row even if degraded', () => {
    const rows = [
      row('kodik', { group: 'ru', state: 'degraded', selectable: true, order: 80 }),
      row('ae', { group: 'firstparty', state: 'degraded', selectable: true, order: 70 }),
    ]
    expect(pickSelectableFallback(rows)?.id).toBe('kodik')
  })

  it('returns null for an empty or non-selectable set', () => {
    expect(pickSelectableFallback([])).toBeNull()
    expect(pickSelectableFallback([row('x', { selectable: false })])).toBeNull()
  })
})
