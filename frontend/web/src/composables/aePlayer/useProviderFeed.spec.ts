import { describe, it, expect } from 'vitest'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
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

  it('hides EN providers when the lang toggle is not EN', () => {
    const rows = rowsFromReport(report, { audio: 'sub', lang: 'ru', content: 'common' })
    expect(rows).toHaveLength(0)
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
})
