import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { Combo } from '@/types/aePlayer'
import type { VerifyReport } from '@/types/contentVerify'
import type { SubPref } from './types'

const h = vi.hoisted(() => ({
  ready: true,
  capGet: vi.fn(),
  animeGet: vi.fn(async (_id: string) => ({ data: { data: { episode_duration: 12 } } })),
  listEpisodes: vi.fn(),
  resolveStream: vi.fn(),
  listDownloads: vi.fn(async () => [] as unknown[]),
  enqueueSeason: vi.fn(async (targets: unknown[], _ctx?: unknown) => targets.length),
  subsAll: vi.fn(async (_id: string, _ep: number) => ({ data: { data: { languages: {}, episode: 1 } } })),
  // One-shot content-verify fetch (Task 15 fix round 4) — defaults to "nothing
  // verified yet" so every pre-existing test keeps its pre-verify (RAW-biased)
  // behavior unless a test explicitly overrides it.
  verifyGet: vi.fn(async (_id: string) => ({ data: { data: { anime_id: 'a1', providers: [] as unknown[] } } })),
}))

vi.mock('./flag', () => ({
  offlineDownloadsEnabled: true,
  offlineRuntimeReady: () => h.ready,
}))
vi.mock('@/api/client', () => ({
  capabilitiesApi: { get: (id: string) => h.capGet(id) },
  animeApi: { getById: (id: string) => h.animeGet(id) },
  subtitlesApi: { all: (id: string, ep: number) => h.subsAll(id, ep) },
  contentVerifyApi: { get: (id: string) => h.verifyGet(id) },
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: (p: string, a: string) => h.listEpisodes(p, a),
    resolveStream: (...args: unknown[]) => h.resolveStream(...args),
    listTeams: async () => [],
  }),
}))
vi.mock('./registry', () => ({
  listDownloads: () => h.listDownloads(),
  // downloadEngine (loaded via the real ./seasonDownload) also imports these:
  putDownload: vi.fn(),
  getDownload: vi.fn(),
  deleteDownloadRecord: vi.fn(),
}))
vi.mock('./seasonDownload', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./seasonDownload')>()
  return {
    ...actual,
    enqueueSeason: (targets: unknown, ctx: unknown) => h.enqueueSeason(targets as unknown[], ctx),
  }
})

import {
  seasonFlow,
  openSeasonDownload,
  confirmSeasonDownload,
  cancelSeasonDownload,
  consumeSeasonNotice,
  pickDefaultCombo,
  _resetSeasonFlowForTests,
} from './seasonDownloadFlow'

function cap(provider: string, group: ProviderCap['group'], audios: string[], order = 10): ProviderCap {
  return {
    provider,
    display_name: provider,
    state: 'active',
    selectable: true,
    hacker_only: false,
    order,
    group,
    audios,
    reason: '',
  } as unknown as ProviderCap
}

function report(...caps: ProviderCap[]): CapabilityReport {
  return { anime_id: 'a1', families: [{ family: 'mixed', providers: caps }] } as unknown as CapabilityReport
}

const ep = (n: number): EpisodeOption => ({ key: `e${n}`, number: n, label: String(n) })

const REQ = { animeId: 'a1', title: 'T', poster: 'p.jpg' }

function envelope(rep: CapabilityReport) {
  return { data: { success: true, data: rep } }
}

beforeEach(() => {
  _resetSeasonFlowForTests()
  h.ready = true
  h.capGet.mockReset()
  h.listEpisodes.mockReset()
  h.resolveStream.mockReset()
  h.listDownloads.mockReset().mockResolvedValue([])
  h.enqueueSeason.mockReset().mockImplementation(async (targets: unknown[]) => targets.length)
  h.subsAll.mockReset().mockResolvedValue({ data: { data: { languages: {}, episode: 1 } } })
  h.verifyGet.mockReset().mockResolvedValue({ data: { data: { anime_id: 'a1', providers: [] } } })
})

// gogoanime verified for DUB/en, kodik verified for DUB/ru — mirrors the
// player's own gating (verifiedCaps.ts): a non-firstparty provider needs a
// matching verified dub_langs entry before it can win a DUB pick.
const VERIFY: VerifyReport = {
  animeId: 'a1',
  providers: {
    gogoanime: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [] },
    kodik: { status: 'verified', raw: true, dub_langs: ['ru'], hardsub_langs: [] },
  },
}

describe('pickDefaultCombo', () => {
  it('prefers DUB in the UI language when content-verify confirms the dub language', () => {
    const rep = report(cap('gogoanime', 'en', ['sub', 'dub'], 10), cap('kodik', 'ru', ['dub'], 20))
    expect(pickDefaultCombo(rep, 'ru', VERIFY)).toMatchObject({ audio: 'dub', lang: 'ru', provider: 'kodik', server: '' })
    expect(pickDefaultCombo(rep, 'en', VERIFY)).toMatchObject({ audio: 'dub', lang: 'en', provider: 'gogoanime' })
  })

  it('falls back to the RAW pick when unverified (no report, or neither provider is confirmed for dub)', () => {
    const rep = report(cap('gogoanime', 'en', ['sub', 'dub'], 10), cap('kodik', 'ru', ['dub'], 20))
    // Neither provider is firstparty; with no verify report DUB is fully
    // gated (effectiveAudios forces ['sub'] regardless of the cap's claimed
    // audios), so RAW wins outright: highest-order row, language filter
    // dropped — kodik (order 20) beats gogoanime (order 10) either way, and
    // its RAW lang is derived from the provider's group (GROUP_PRIMARY_LANG),
    // not the requested uiLang.
    expect(pickDefaultCombo(rep, 'ru')).toMatchObject({ audio: 'sub', lang: 'ru', provider: 'kodik', server: '' })
    expect(pickDefaultCombo(rep, 'en')).toMatchObject({ audio: 'sub', lang: 'ru', provider: 'kodik' })
    // Same outcome passing verify=null explicitly (the fetch-failure shape).
    expect(pickDefaultCombo(rep, 'ru', null)).toMatchObject({ audio: 'sub', provider: 'kodik' })
  })

  it('falls back to RAW with the provider-group served lang (firstparty exempt from gating)', () => {
    const rep = report(cap('ae', 'firstparty', ['sub'], 10))
    expect(pickDefaultCombo(rep, 'en')).toMatchObject({ audio: 'sub', lang: 'ja', provider: 'ae' })
  })

  it('returns null on an empty/malformed report', () => {
    expect(pickDefaultCombo(null, 'en')).toBeNull()
    expect(pickDefaultCombo({ anime_id: 'a1', families: [] } as unknown as CapabilityReport, 'en')).toBeNull()
  })
})

describe('openSeasonDownload', () => {
  it('resolves caps + episodes, skips already-downloaded, lands in choose', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    h.listDownloads.mockResolvedValue([{ animeId: 'a1', episode: { number: 1 }, state: 'done' }])
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
    expect(seasonFlow.targets.map((e) => e.number)).toEqual([2, 3])
    expect(h.listEpisodes).toHaveBeenCalledWith('gogoanime', 'a1')
  })

  it('a verified content-verify report lets the resolved combo land on the DUB pick', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']), cap('kodik', 'ru', ['dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    h.verifyGet.mockResolvedValue({
      data: {
        data: {
          anime_id: 'a1',
          providers: [
            { provider: 'gogoanime', summary: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [] }, units: [] },
          ],
        },
      },
    })
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
    expect(seasonFlow.combo).toMatchObject({ audio: 'dub', lang: 'en', provider: 'gogoanime' })
    expect(h.listEpisodes).toHaveBeenCalledWith('gogoanime', 'a1')
  })

  it('a content-verify fetch failure degrades gracefully to the RAW pick (does not fail the whole flow)', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    h.verifyGet.mockRejectedValue(new Error('network'))
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
    expect(seasonFlow.combo).toMatchObject({ audio: 'sub', provider: 'gogoanime' })
    expect(seasonFlow.notice).toBeNull() // no 'failed' notice — the verify fetch is best-effort only
  })

  it('notices no-sw without touching the network', async () => {
    h.ready = false
    await openSeasonDownload(REQ, 'en')
    expect(consumeSeasonNotice()).toEqual({ kind: 'no-sw' })
    expect(h.capGet).not.toHaveBeenCalled()
  })

  it('notices nothing-left when every episode is stored', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['dub']))))
    h.listEpisodes.mockResolvedValue([ep(1)])
    h.listDownloads.mockResolvedValue([{ animeId: 'a1', episode: { number: 1 }, state: 'done' }])
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('idle')
    expect(consumeSeasonNotice()).toEqual({ kind: 'nothing-left' })
  })

  it('notices no-source when the feed offers nothing', async () => {
    h.capGet.mockResolvedValue(envelope({ anime_id: 'a1', families: [] } as unknown as CapabilityReport))
    await openSeasonDownload(REQ, 'en')
    expect(consumeSeasonNotice()).toEqual({ kind: 'no-source' })
  })

  it('cancel during resolve discards the in-flight result', async () => {
    let release!: (v: unknown) => void
    h.capGet.mockReturnValue(new Promise((r) => (release = r)))
    const p = openSeasonDownload(REQ, 'en')
    cancelSeasonDownload()
    release(envelope(report(cap('gogoanime', 'en', ['dub']))))
    await p
    expect(seasonFlow.phase).toBe('idle')
    expect(seasonFlow.notice).toBeNull()
  })

  it('open() stores the report; subTracks populated from first-target subtitle fetch', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    h.subsAll.mockResolvedValue({
      data: {
        data: {
          languages: { ja: [{ url: 'u.ass', lang: 'ja', label: 'Sub', format: 'ass', provider: 'jimaku' }] },
          episode: 1,
        },
      },
    })
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
    expect(seasonFlow.report).not.toBeNull()
    expect(seasonFlow.subTracks).toHaveLength(1)
    expect(seasonFlow.subTracks[0]).toMatchObject({ lang: 'ja', provider: 'jimaku' })
  })

  it('open() tolerates subtitle fetch failure — subTracks empty, phase still choose', async () => {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    h.subsAll.mockRejectedValue(new Error('network'))
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
    expect(seasonFlow.subTracks).toEqual([])
  })
})

describe('confirmSeasonDownload', () => {
  async function toChoose() {
    h.capGet.mockResolvedValue(envelope(report(cap('gogoanime', 'en', ['sub', 'dub']))))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2), ep(3)])
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')
  }

  it('enqueues every target with the frozen combo', async () => {
    await toChoose()
    await confirmSeasonDownload('720')
    expect(h.enqueueSeason).toHaveBeenCalledTimes(1)
    const [targets, ctx] = h.enqueueSeason.mock.calls[0] as unknown as [EpisodeOption[], Record<string, unknown>]
    expect(targets.map((e) => e.number)).toEqual([1, 2, 3])
    expect(ctx).toMatchObject({ animeId: 'a1', animeTitle: 'T', poster: 'p.jpg', quality: '720', durationMin: 12 })
    expect(consumeSeasonNotice()).toEqual({ kind: 'queued', n: 3 })
    expect(seasonFlow.phase).toBe('idle')
  })

  it('confirm with a DIFFERENT provider re-lists episodes and recomputes targets', async () => {
    await toChoose()
    // Override listEpisodes for the kodik provider re-list
    h.listEpisodes.mockResolvedValue([ep(1), ep(2)])
    const kodikCombo: Combo = { ...(seasonFlow.combo as Combo), provider: 'kodik' }
    await confirmSeasonDownload('720', kodikCombo, null)
    expect(h.listEpisodes).toHaveBeenCalledWith('kodik', 'a1')
    const [targets, ctx] = h.enqueueSeason.mock.calls[0] as [EpisodeOption[], Record<string, unknown>]
    expect(targets.map((e) => e.number)).toEqual([1, 2])
    expect((ctx as { combo: Combo }).combo.provider).toBe('kodik')
  })

  it('confirm threads subPref + resolveSubsFor into enqueueSeason ctx', async () => {
    await toChoose()
    const pref: SubPref = { kind: 'external', provider: 'jimaku', lang: 'ja' }
    await confirmSeasonDownload('720', null, pref)
    const [, ctx] = h.enqueueSeason.mock.calls[0] as [EpisodeOption[], Record<string, unknown>]
    expect((ctx as { subPref: SubPref }).subPref).toEqual(pref)
    expect(typeof (ctx as { resolveSubsFor?: unknown }).resolveSubsFor).toBe('function')
  })

  it('cancel during confirm re-list does not enqueue after cancel', async () => {
    await toChoose()

    // Deferred promise for the re-list triggered by the provider change
    let release!: (v: EpisodeOption[]) => void
    h.listEpisodes.mockReturnValueOnce(new Promise<EpisodeOption[]>((r) => (release = r)))

    const kodikCombo: Combo = { ...(seasonFlow.combo as Combo), provider: 'kodik' }
    const p = confirmSeasonDownload('720', kodikCombo, null)

    // Cancel while listEpisodes is still in flight
    cancelSeasonDownload()

    // Resolve the deferred after cancellation
    release([ep(1), ep(2)])
    await p

    // enqueueSeason must NOT have been called
    expect(h.enqueueSeason).not.toHaveBeenCalled()
    // State must remain as cancel left it: idle + no 'queued' notice stomp
    expect(seasonFlow.phase).toBe('idle')
    expect(seasonFlow.notice).toBeNull()
  })

  it('cancel during confirm re-list REJECT does not stomp state with a failed notice', async () => {
    await toChoose()

    // Deferred reject for the re-list triggered by the provider change
    let rejectDeferred!: (e: Error) => void
    h.listEpisodes.mockReturnValueOnce(
      new Promise<EpisodeOption[]>((_, r) => (rejectDeferred = r)),
    )

    const kodikCombo: Combo = { ...(seasonFlow.combo as Combo), provider: 'kodik' }
    const p = confirmSeasonDownload('720', kodikCombo, null)

    // Cancel bumps seq before the reject lands
    cancelSeasonDownload()

    // Now reject the deferred — without the guard this would call reset({kind:'failed'})
    rejectDeferred(new Error('network error'))
    await p

    // The failed reset must be suppressed; state stays as cancel left it
    expect(seasonFlow.phase).toBe('idle')
    expect(consumeSeasonNotice()).toBeNull()
  })
})
