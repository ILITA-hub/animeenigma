import { reactive, readonly } from 'vue'
import type { Combo, AudioKind, TrackLang, ContentKind, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { CapabilityReport } from '@/types/capabilities'
import { capabilitiesApi, animeApi, subtitlesApi } from '@/api/client'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import { pickSmartDefault, pickSelectableFallback } from '@/composables/aePlayer/smartDefault'
import { GROUP_PRIMARY_LANG } from '@/composables/aePlayer/providerGroups'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import { flattenAggregateSubs, type AggregateSubsResponse } from '@/composables/aePlayer/useSubtitleTracks'
import { makeExternalSubResolver } from './externalSubs'
import { seasonTargets, enqueueSeason } from './seasonDownload'
import { listDownloads } from './registry'
import { offlineRuntimeReady } from './flag'
import type { DownloadState, SubPref } from './types'

// Season download launched OUTSIDE the player (anime-card context menu).
// Unlike the in-player path there is no user-picked combo, so this flow
// resolves the capability feed and picks the BEST default source the same way
// the player's smart default does, lists episodes through the same resolver,
// then feeds the same serial download engine. State is a module-level
// singleton rendered by <SeasonDownloadHost /> (mounted once in App.vue).

export interface SeasonDownloadRequest {
  animeId: string
  title: string
  poster?: string
}

export type SeasonFlowNotice =
  | { kind: 'no-sw' | 'no-source' | 'nothing-left' }
  | { kind: 'failed'; message?: string }
  | { kind: 'queued'; n: number }

interface SeasonFlowState {
  phase: 'idle' | 'resolving' | 'choose' | 'queueing'
  request: SeasonDownloadRequest | null
  targets: EpisodeOption[]
  combo: Combo | null
  /** Episode runtime (minutes) from the catalog detail — scales size estimates. */
  durationMin: number | null
  /** Capability report for the source combo picker (passed to DownloadDialog). */
  report: CapabilityReport | null
  /** External (aggregated) tracks for the first target — the dialog's subtitle menu. */
  subTracks: SubtitleTrack[]
  /** One-shot result; the host turns it into a toast via consumeSeasonNotice(). */
  notice: SeasonFlowNotice | null
}

const state = reactive<SeasonFlowState>({
  phase: 'idle',
  request: null,
  targets: [],
  combo: null,
  durationMin: null,
  report: null,
  subTracks: [],
  notice: null,
})

// Cancellation token: cancel() bumps it so a resolve still in flight discards
// its result instead of reviving a dismissed dialog.
let seq = 0

/** Per-anime download states keyed by episode number — the seasonTargets input. */
async function animeDownloadStates(animeId: string): Promise<Record<number, DownloadState>> {
  const states: Record<number, DownloadState> = {}
  for (const d of await listDownloads()) if (d.animeId === animeId) states[d.episode.number] = d.state
  return states
}

function reset(notice: SeasonFlowNotice | null): void {
  state.phase = 'idle'
  state.request = null
  state.targets = []
  state.combo = null
  state.durationMin = null
  state.report = null
  state.subTracks = []
  state.notice = notice
}

/** Player-default parity: DUB in the UI language, then the other DUB, then RAW
 *  (which drops the lang filter); hentai content only when common yields no rows. */
export function pickDefaultCombo(report: CapabilityReport | null, uiLang: string): Combo | null {
  const langPref: TrackLang = uiLang.startsWith('ru') ? 'ru' : 'en'
  const altLang: TrackLang = langPref === 'ru' ? 'en' : 'ru'
  const candidates: { audio: AudioKind; lang: TrackLang }[] = [
    { audio: 'dub', lang: langPref },
    { audio: 'dub', lang: altLang },
    { audio: 'sub', lang: 'en' },
  ]
  for (const content of ['common', 'hentai'] as ContentKind[]) {
    for (const c of candidates) {
      const rows = rowsFromReport(report, { audio: c.audio, lang: c.lang, content })
      const row = pickSmartDefault(rows) ?? pickSelectableFallback(rows)
      if (row) {
        // Under RAW the served lang is derived from the provider's group,
        // mirroring the player's setServedLang behavior.
        const lang = c.audio === 'sub' ? GROUP_PRIMARY_LANG[row.group] : c.lang
        return { audio: c.audio, lang, provider: row.id, server: '', team: null }
      }
    }
  }
  return null
}

export async function openSeasonDownload(request: SeasonDownloadRequest, uiLang: string): Promise<void> {
  if (!offlineRuntimeReady()) {
    state.notice = { kind: 'no-sw' }
    return
  }
  if (state.phase !== 'idle') return
  const mySeq = ++seq
  state.phase = 'resolving'
  state.request = request
  try {
    // Detail fetch rides along only for episode_duration (scales the size
    // estimates); its failure must never block the download flow.
    const [res, durationMin] = await Promise.all([
      capabilitiesApi.get(request.animeId),
      animeApi
        .getById(request.animeId)
        .then((r) => {
          const detail = (r.data?.data ?? r.data ?? null) as { episode_duration?: number } | null
          const d = detail?.episode_duration
          return typeof d === 'number' && d > 0 ? d : null
        })
        .catch(() => null),
    ])
    if (mySeq !== seq) return
    const report = (res.data?.data ?? res.data ?? null) as CapabilityReport | null
    const combo = pickDefaultCombo(report, uiLang)
    if (!combo) {
      reset({ kind: 'no-source' })
      return
    }
    const resolver = useProviderResolver()
    const episodes = await resolver.listEpisodes(combo.provider, request.animeId)
    if (mySeq !== seq) return
    const states = await animeDownloadStates(request.animeId)
    if (mySeq !== seq) return
    const targets = seasonTargets(episodes, states)
    if (targets.length === 0) {
      reset({ kind: 'nothing-left' })
      return
    }
    // External subtitle menu rides along; its failure must never block the flow.
    const subTracks = await subtitlesApi
      .all(request.animeId, targets[0].number)
      .then((r) => flattenAggregateSubs((r.data?.data ?? r.data) as AggregateSubsResponse))
      .catch(() => [] as SubtitleTrack[])
    if (mySeq !== seq) return
    state.report = report
    state.subTracks = subTracks
    state.targets = targets
    state.combo = combo
    state.durationMin = durationMin
    state.phase = 'choose'
  } catch (e) {
    console.error('[seasonDownload] resolve failed', e)
    if (mySeq === seq) reset({ kind: 'failed', message: e instanceof Error ? e.message : String(e) })
  }
}

export async function confirmSeasonDownload(
  quality: string,
  combo?: Combo | null,
  subPref?: SubPref | null,
): Promise<void> {
  const req = state.request
  const chosen = combo ? { ...combo } : state.combo ? { ...state.combo } : null
  if (!req || !chosen || state.phase !== 'choose') return
  state.phase = 'queueing'
  const mySeq = seq
  const resolver = useProviderResolver()
  try {
    // The episode list came from the DEFAULT provider — a dialog-picked
    // provider numbers episodes with its own keys, so re-list and re-filter.
    let targets = state.targets
    if (chosen.provider !== state.combo?.provider) {
      const episodes = await resolver.listEpisodes(chosen.provider, req.animeId)
      if (mySeq !== seq) return
      const states = await animeDownloadStates(req.animeId)
      if (mySeq !== seq) return
      targets = seasonTargets(episodes, states)
      if (targets.length === 0) {
        reset({ kind: 'nothing-left' })
        return
      }
    }
    const eps = targets.map((ep) => ({ ...ep })) // de-proxy before IndexedDB
    const n = await enqueueSeason(eps, {
      animeId: req.animeId,
      animeTitle: req.title,
      poster: req.poster,
      combo: chosen,
      quality,
      durationMin: state.durationMin ?? undefined,
      subPref: subPref ?? undefined,
      resolveSubsFor: makeExternalSubResolver(req.animeId, subPref),
      resolveFor: (ep) => () => resolver.resolveStream(chosen.provider, req.animeId, ep, chosen),
    })
    reset({ kind: 'queued', n })
  } catch (e) {
    console.error('[seasonDownload] confirm failed', e)
    if (mySeq === seq) reset({ kind: 'failed', message: e instanceof Error ? e.message : String(e) })
  }
}

export function cancelSeasonDownload(): void {
  seq++
  reset(null)
}

/** Host reads the pending notice exactly once (toast), then it is cleared. */
export function consumeSeasonNotice(): SeasonFlowNotice | null {
  const n = state.notice
  state.notice = null
  return n
}

export const seasonFlow = readonly(state)

export function _resetSeasonFlowForTests(): void {
  seq++
  reset(null)
}
