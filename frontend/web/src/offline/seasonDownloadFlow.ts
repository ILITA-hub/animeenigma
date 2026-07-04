import { reactive, readonly } from 'vue'
import type { Combo, AudioKind, TrackLang, ContentKind } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { CapabilityReport } from '@/types/capabilities'
import { capabilitiesApi, animeApi } from '@/api/client'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import { pickSmartDefault, pickSelectableFallback } from '@/composables/aePlayer/smartDefault'
import { GROUP_PRIMARY_LANG } from '@/composables/aePlayer/providerGroups'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import { seasonTargets, enqueueSeason } from './seasonDownload'
import { listDownloads } from './registry'
import { offlineRuntimeReady } from './flag'
import type { DownloadState } from './types'

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
  /** One-shot result; the host turns it into a toast via consumeSeasonNotice(). */
  notice: SeasonFlowNotice | null
}

const state = reactive<SeasonFlowState>({
  phase: 'idle',
  request: null,
  targets: [],
  combo: null,
  durationMin: null,
  notice: null,
})

// Cancellation token: cancel() bumps it so a resolve still in flight discards
// its result instead of reviving a dismissed dialog.
let seq = 0

function reset(notice: SeasonFlowNotice | null): void {
  state.phase = 'idle'
  state.request = null
  state.targets = []
  state.combo = null
  state.durationMin = null
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
    const all = await listDownloads()
    if (mySeq !== seq) return
    const states: Record<number, DownloadState> = {}
    for (const d of all) if (d.animeId === request.animeId) states[d.episode.number] = d.state
    const targets = seasonTargets(episodes, states)
    if (targets.length === 0) {
      reset({ kind: 'nothing-left' })
      return
    }
    state.targets = targets
    state.combo = combo
    state.durationMin = durationMin
    state.phase = 'choose'
  } catch (e) {
    console.error('[seasonDownload] resolve failed', e)
    if (mySeq === seq) reset({ kind: 'failed', message: e instanceof Error ? e.message : String(e) })
  }
}

export async function confirmSeasonDownload(quality: string, scope: 'episode' | 'season'): Promise<void> {
  const req = state.request
  const combo = state.combo ? { ...state.combo } : null
  if (!req || !combo || state.phase !== 'choose') return
  // Plain copies: `state` is reactive, so its elements are Proxies — IndexedDB
  // structured clone rejects those (DataCloneError). The engine de-proxies too;
  // this keeps the flow safe even against future engine callers.
  const picked = scope === 'season' ? state.targets : state.targets.slice(0, 1)
  const eps = picked.map((ep) => ({ ...ep }))
  state.phase = 'queueing'
  const resolver = useProviderResolver()
  try {
    const n = await enqueueSeason(eps, {
      animeId: req.animeId,
      animeTitle: req.title,
      poster: req.poster,
      combo,
      quality,
      durationMin: state.durationMin ?? undefined,
      resolveFor: (ep) => () => resolver.resolveStream(combo.provider, req.animeId, ep, combo),
    })
    reset({ kind: 'queued', n })
  } catch (e) {
    console.error('[seasonDownload] enqueue failed', e)
    reset({ kind: 'failed', message: e instanceof Error ? e.message : String(e) })
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
