import { describe, it, expect, vi } from 'vitest'
import type { SubtitleTrack } from '@/types/aePlayer'
import { matchAutoSub, makeExternalSubResolver } from './externalSubs'

vi.mock('@/api/client', () => ({
  subtitlesApi: {
    all: vi.fn(async (_id: string, ep: number) => ({
      data: { data: { languages: { ja: [
        { url: `/sub/jimaku-${ep}.ass`, lang: 'ja', label: 'Jimaku A', provider: 'jimaku', format: 'ass' },
        { url: `/sub/jimaku2-${ep}.srt`, lang: 'ja', label: 'Jimaku B', provider: 'jimaku' },
      ] }, episode: ep } },
    })),
  },
}))

const tr = (p: Partial<SubtitleTrack>): SubtitleTrack =>
  ({ url: 'u', provider: 'gogoanime', lang: 'en', label: 'L', format: 'vtt', ...p })

describe('matchAutoSub', () => {
  const subs = [
    tr({ url: '/__offline/x/sub/0', provider: 'gogoanime', lang: 'en' }),
    tr({ url: '/__offline/x/sub/1', provider: 'gogoanime', lang: 'ru' }),
    tr({ url: '/__offline/x/sub/2', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' }),
  ]
  it('no pref → undefined', () => expect(matchAutoSub(undefined, subs, 'gogoanime')).toBeUndefined())
  it('bundled auto → first stream-provider track', () =>
    expect(matchAutoSub({ kind: 'bundled', lang: 'auto' }, subs, 'gogoanime')).toBe('/__offline/x/sub/0'))
  it('bundled concrete lang → lang match only among bundled', () => {
    expect(matchAutoSub({ kind: 'bundled', lang: 'ru' }, subs, 'gogoanime')).toBe('/__offline/x/sub/1')
    expect(matchAutoSub({ kind: 'bundled', lang: 'ja' }, subs, 'gogoanime')).toBeUndefined()
  })
  it('external → provider+lang (label preferred)', () =>
    expect(matchAutoSub({ kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' }, subs, 'gogoanime'))
      .toBe('/__offline/x/sub/2'))
})

describe('makeExternalSubResolver', () => {
  it('non-external pref → undefined', () => {
    expect(makeExternalSubResolver('a1', null)).toBeUndefined()
    expect(makeExternalSubResolver('a1', { kind: 'bundled', lang: 'auto' })).toBeUndefined()
  })
  it('fetches per-episode and matches by provider+lang+label', async () => {
    const forEp = makeExternalSubResolver('a1', { kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku B' })!
    const got = await forEp({ key: 5, label: 5, number: 5 })()
    expect(got).toHaveLength(1)
    expect(got[0].url).toBe('/sub/jimaku2-5.srt')
  })
  it('no match for the episode → empty list', async () => {
    const forEp = makeExternalSubResolver('a1', { kind: 'external', provider: 'opensubtitles', lang: 'en' })!
    expect(await forEp({ key: 5, label: 5, number: 5 })()).toEqual([])
  })
})
