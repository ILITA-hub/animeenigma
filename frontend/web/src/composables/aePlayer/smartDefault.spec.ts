import { describe, it, expect } from 'vitest'
import { pickSmartDefault, pickRawBiased, pickSelectableFallback, defaultPool } from './smartDefault'
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

  // Phase C source-panel truth: a row's real per-title `lang` (set only for
  // ae's probed dub variant) must gate the language match, not the
  // `firstparty` group's full nominal set (en/ru/ja). Before the fix this used
  // GROUP_LANGS[row.group] directly, so a higher-order ae en-dub row would
  // wrongly win a `ru` RAW-biased pick over a real ru source (kodik).
  it("excludes a row whose real per-title lang doesn't match, even though its group nominally serves it", () => {
    const rows = [
      row('ae', { group: 'firstparty', lang: 'en', order: 90 }),
      row('kodik', { group: 'ru', order: 80 }),
    ]
    expect(pickRawBiased(rows, 'ru')?.id).toBe('kodik')
  })
})

describe('defaultPool', () => {
  // ae is auto-cached and can be PARTIAL — sometimes only a late episode (e.g.
  // Frieren ep 27 of 28), flagged by the backend as `partialLibrary`. On a fresh
  // open with no requested episode we want to land on episode 1 of a full
  // source, so a PARTIAL ae (firstparty) is dropped from the auto-default when a
  // real alternative exists. A COMPLETE ae library (no flag) stays the default;
  // a specified episode (resume / deep-link) keeps even a partial ae eligible.
  const partialAe = (over: Partial<ProviderRow> = {}) =>
    row('ae', { group: 'firstparty', order: 100, partialLibrary: true, ...over })
  const completeAe = (over: Partial<ProviderRow> = {}) =>
    row('ae', { group: 'firstparty', order: 100, ...over })

  it('keeps a partial ae when an episode IS specified (resume / deep-link)', () => {
    const rows = [partialAe(), row('animejoy-sibnet', { group: 'ru', order: 25 })]
    expect(defaultPool(rows, true).map((r) => r.id)).toContain('ae')
  })

  it('drops a PARTIAL ae for an unspecified open when an active alternative exists', () => {
    const rows = [partialAe(), row('animejoy-sibnet', { group: 'ru', order: 25 })]
    const pool = defaultPool(rows, false)
    expect(pool.map((r) => r.id)).not.toContain('ae')
    expect(pool.map((r) => r.id)).toContain('animejoy-sibnet')
  })

  it('KEEPS a COMPLETE ae (no partialLibrary flag) as the default for a fresh open', () => {
    const rows = [completeAe(), row('animejoy-sibnet', { group: 'ru', order: 25 })]
    expect(defaultPool(rows, false).map((r) => r.id)).toContain('ae')
  })

  it('keeps a partial ae when it is the only ACTIVE selectable source (no alternative)', () => {
    const rows = [
      partialAe(),
      row('gogoanime', { state: 'degraded', selectable: false, order: 85 }),
    ]
    expect(defaultPool(rows, false).map((r) => r.id)).toContain('ae')
  })

  it('Frieren shape: fresh RAW/sub open picks the full animejoy source, not the partial ae', () => {
    const rows = [
      partialAe({ audios: ['sub'] }), // library holds only ep 27
      row('animejoy-sibnet', { group: 'ru', order: 25, audios: ['sub'] }),
      row('gogoanime', { state: 'degraded', selectable: false, order: 85 }),
    ]
    // Under RAW the language slider is hidden; ja has no in-group source, so the
    // pick falls through to the global best of the ae-free pool.
    expect(pickRawBiased(defaultPool(rows, false), 'ja')?.id).toBe('animejoy-sibnet')
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
