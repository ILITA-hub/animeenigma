import { describe, it, expect } from 'vitest'
import { rowsFromReport, groupOfProvider } from '@/composables/aePlayer/useProviderFeed'
import type { CapabilityReport } from '@/types/capabilities'

const report: CapabilityReport = {
  anime_id: 'x',
  families: [
    { family: 'ourenglish', providers: [
      { provider: 'gogoanime', display_name: 'GogoAnime', state: 'active', selectable: true,
        hacker_only: false, order: 85, group: 'en', audios: ['sub', 'dub'], variants: [] },
      { provider: 'animefever', display_name: 'AnimeFever', state: 'degraded', selectable: true,
        hacker_only: true, order: 60, group: 'en', audios: ['sub'], variants: [], reason: 'ads' },
    ] },
  ],
} as unknown as CapabilityReport

describe('rowsFromReport', () => {
  it('flattens, sorts by order desc, carries state', () => {
    const rows = rowsFromReport(report, { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'animefever'])
    expect(rows[0].state).toBe('active')
    expect(rows[1].hackerOnly).toBe(true)
    expect(rows[1].reason).toBe('ads')
  })

  it('disabled providers never appear (backend omits them)', () => {
    // animepahe is policy=disabled → absent from the report → absent from rows
    expect(rowsFromReport(report, { audio: 'sub', lang: 'en', content: 'common' })
      .find(r => r.id === 'animepahe')).toBeUndefined()
  })

  it('filters out providers that cannot serve the active audio/lang combo', () => {
    // dub filter → animefever (sub-only) drops, gogoanime (sub+dub) stays
    const rows = rowsFromReport(report, { audio: 'dub', lang: 'en', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime'])
  })

  it('RAW ignores the language filter — EN sources show even when lang is not EN', () => {
    // Under RAW (audio:'sub') the language slider is hidden; original-audio
    // sources surface regardless of the lang value carried in the combo.
    const rows = rowsFromReport(report, { audio: 'sub', lang: 'ru', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'animefever'])
  })

  it('RAW lists en and ru original sources (sub caps) and drops dub-only', () => {
    const multi = {
      anime_id: 'm',
      families: [
        { family: 'ourenglish', providers: [
          { provider: 'gogoanime', display_name: 'GogoAnime', state: 'active', selectable: true,
            hacker_only: false, order: 90, group: 'en', audios: ['sub', 'dub'], variants: [] },
        ] },
        { family: 'ru', providers: [
          { provider: 'kodik', display_name: 'Kodik', state: 'active', selectable: true,
            hacker_only: false, order: 80, group: 'ru', audios: ['sub', 'dub'], variants: [] },
        ] },
        { family: 'en', providers: [
          { provider: 'dubonly', display_name: 'DubOnly', state: 'active', selectable: true,
            hacker_only: false, order: 60, group: 'en', audios: ['dub'], variants: [] },
        ] },
      ],
    } as unknown as CapabilityReport
    const rows = rowsFromReport(multi, { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'kodik'])
  })

  it('DUB keeps the language gate (en vs ru)', () => {
    const multi = {
      anime_id: 'm',
      families: [
        { family: 'ourenglish', providers: [
          { provider: 'gogoanime', display_name: 'GogoAnime', state: 'active', selectable: true,
            hacker_only: false, order: 90, group: 'en', audios: ['sub', 'dub'], variants: [] },
        ] },
        { family: 'ru', providers: [
          { provider: 'kodik', display_name: 'Kodik', state: 'active', selectable: true,
            hacker_only: false, order: 80, group: 'ru', audios: ['sub', 'dub'], variants: [] },
        ] },
      ],
    } as unknown as CapabilityReport
    expect(rowsFromReport(multi, { audio: 'dub', lang: 'en', content: 'common' }).map(r => r.id)).toEqual(['gogoanime'])
    expect(rowsFromReport(multi, { audio: 'dub', lang: 'ru', content: 'common' }).map(r => r.id)).toEqual(['kodik'])
  })

  it('keeps 18+ sources visible on a hentai title regardless of audio/lang', () => {
    const hentaiReport = {
      anime_id: 'h',
      families: [
        { family: 'hanime', providers: [
          { provider: 'hanime', display_name: 'Hanime', state: 'active', selectable: true,
            hacker_only: false, order: 50, group: 'adult', audios: ['dub'], variants: [] },
        ] },
      ],
    } as unknown as CapabilityReport
    // sub/en filter, but a hentai title → adult source still shows
    const rows = rowsFromReport(hentaiReport, { audio: 'sub', lang: 'en', content: 'hentai' })
    expect(rows.map(r => r.id)).toEqual(['hanime'])
  })

  it('yields an empty list for a null/malformed report', () => {
    expect(rowsFromReport(null, { audio: 'sub', lang: 'en', content: 'common' })).toEqual([])
  })

  // Phase C source-panel truth: a cap's real per-title `lang` (set only for
  // ae's probed dub variant) must narrow the DUB relevance gate to that exact
  // language, not the `firstparty` group's full nominal set (en/ru/ja). Before
  // the fix, `relevant()` read GROUP_LANGS[cap.group] directly, so an ae
  // English dub would wrongly satisfy DUB+RU and DUB+JA too.
  it('an ae en-dub cap (cap.lang) is included under DUB+EN and excluded under DUB+RU', () => {
    const aeReport = {
      anime_id: 'ae-title',
      families: [
        { family: 'aeProvider', providers: [
          { provider: 'ae', display_name: 'AnimeEnigma', state: 'active', selectable: true,
            hacker_only: false, order: 100, group: 'firstparty', audios: ['dub'], lang: 'en', variants: [] },
        ] },
      ],
    } as unknown as CapabilityReport
    expect(rowsFromReport(aeReport, { audio: 'dub', lang: 'en', content: 'common' }).map(r => r.id)).toEqual(['ae'])
    expect(rowsFromReport(aeReport, { audio: 'dub', lang: 'ru', content: 'common' })).toEqual([])
  })
})

const groupReport = {
  anime_id: 'x',
  families: [
    { family: 'others', providers: [
      { provider: 'gogoanime', group: 'en' }, { provider: 'allanime-okru', group: 'en' },
      { provider: 'kodik', group: 'ru' },
    ] },
  ],
} as unknown as CapabilityReport

describe('groupOfProvider', () => {
  it('returns the group for a known provider', () => {
    expect(groupOfProvider(groupReport, 'allanime-okru')).toBe('en')
    expect(groupOfProvider(groupReport, 'kodik')).toBe('ru')
  })
  it('returns undefined for an unknown provider or null report', () => {
    expect(groupOfProvider(groupReport, 'nope')).toBeUndefined()
    expect(groupOfProvider(null, 'gogoanime')).toBeUndefined()
  })
})
