import { describe, it, expect } from 'vitest'
import { rowsFromReport, groupOfProvider } from '@/composables/aePlayer/useProviderFeed'
import type { CapabilityReport } from '@/types/capabilities'
import type { VerifyReport } from '@/types/contentVerify'

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

// Three-provider report shared by the RAW/DUB gating tests below: gogoanime
// and kodik both nominally claim sub+dub; dubonly claims dub only.
const multiWithDubOnly = {
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

const multiTwoGroups = {
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

  // Owner-approved gate (content-verify spec §5): a non-firstparty cap with no
  // verify row is assumed RAW-only. DUB never surfaces such a provider, no
  // matter what its cap.audios claims, until the probe confirms a dub lang.
  it('unverified non-firstparty caps never satisfy DUB — no verify report means no DUB rows', () => {
    const rows = rowsFromReport(report, { audio: 'dub', lang: 'en', content: 'common' })
    expect(rows).toEqual([])
  })

  it('DUB surfaces a provider once content-verify confirms a matching dub language', () => {
    const verify: VerifyReport = {
      animeId: 'x',
      providers: { gogoanime: { status: 'verified', raw: false, dub_langs: ['en'], hardsub_langs: [] } },
    }
    const rows = rowsFromReport(report, { audio: 'dub', lang: 'en', content: 'common' }, verify)
    expect(rows.map(r => r.id)).toEqual(['gogoanime'])
  })

  it('RAW ignores the language filter — EN sources show even when lang is not EN', () => {
    // Under RAW (audio:'sub') the language slider is hidden; original-audio
    // sources surface regardless of the lang value carried in the combo.
    const rows = rowsFromReport(report, { audio: 'sub', lang: 'ru', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'animefever'])
  })

  it('RAW lists every unverified non-firstparty source regardless of claimed audios (RAW-assumed default)', () => {
    // dubonly nominally claims dub-only, but with no verify row content-verify
    // treats it as unproven — assumed RAW like everything else, tagged
    // "unverified" (see ProviderChip). No claims without verification.
    const rows = rowsFromReport(multiWithDubOnly, { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'kodik', 'dubonly'])
  })

  it('a verified dub-only provider drops out of RAW once the probe proves it has no raw audio', () => {
    const verify: VerifyReport = {
      animeId: 'm',
      providers: { dubonly: { status: 'verified', raw: false, dub_langs: ['en'], hardsub_langs: [] } },
    }
    const rows = rowsFromReport(multiWithDubOnly, { audio: 'sub', lang: 'en', content: 'common' }, verify)
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'kodik'])
  })

  it('DUB keeps the language gate (en vs ru) once each provider has a verified dub lang', () => {
    const verify: VerifyReport = {
      animeId: 'm',
      providers: {
        gogoanime: { status: 'verified', raw: false, dub_langs: ['en'], hardsub_langs: [] },
        kodik: { status: 'verified', raw: false, dub_langs: ['ru'], hardsub_langs: [] },
      },
    }
    expect(rowsFromReport(multiTwoGroups, { audio: 'dub', lang: 'en', content: 'common' }, verify).map(r => r.id)).toEqual(['gogoanime'])
    expect(rowsFromReport(multiTwoGroups, { audio: 'dub', lang: 'ru', content: 'common' }, verify).map(r => r.id)).toEqual(['kodik'])
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
  // language, not the `firstparty` group's full nominal set (en/ru/ja).
  // `firstparty` is also exempt from the content-verify gate — it trusts
  // cap.audios/cap.lang as-is (first-party ingest truth) even with no verify row.
  it('an ae en-dub cap (cap.lang) is included under DUB+EN and excluded under DUB+RU — firstparty is exempt from the verify gate', () => {
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
