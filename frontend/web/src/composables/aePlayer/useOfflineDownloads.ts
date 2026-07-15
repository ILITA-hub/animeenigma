import { computed, onMounted, ref, watch, type Ref } from 'vue'
import { offlineDownloadsEnabled, offlineRuntimeReady } from '@/offline/flag'
import { engineState } from '@/offline/downloadEngine'
import { useDownloadGate } from '@/offline/downloadGate'
import { seasonTargets, enqueueSeason } from '@/offline/seasonDownload'
import { listDownloads } from '@/offline/registry'
import { makeExternalSubResolver, externalSubOptions } from '@/offline/externalSubs'
import type { OfflinePlayback } from '@/offline/offlineAdapter'
import type { DownloadState, SubPref, SubOption } from '@/offline/types'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { ProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import type { useToast } from '@/composables/useToast'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { AudioKind, Combo } from '@/types/aePlayer'

// ── Offline downloads (season-only, app-only) ────────────────────────────────

export interface OfflineDownloadsDeps {
  getOffline: () => OfflinePlayback | null | undefined
  getAnimeId: () => string
  getAnimeMeta: () => { title: string; still?: string; durationMin?: number }
  state: PlayerState
  resolver: ProviderResolver
  episodes: Ref<EpisodeOption[]>
  downloadDialogOpen: Ref<boolean>
  toast: ReturnType<typeof useToast>
  t: (key: string) => string
  /** Late-bound: the subtitle cluster composes after this one. */
  getSubtitleTracks: () => Array<{ url: string; lang: string; label: string; provider: string; format: string }>
  getBundledTracks: () => Array<{ url: string; lang: string }>
  ensureSubsLoaded: () => Promise<void> | void
}

export function useOfflineDownloads(deps: OfflineDownloadsDeps) {
  const { state, resolver, episodes, downloadDialogOpen, toast, t } = deps

  const downloadStates = ref<Record<number, DownloadState>>({})
  const seasonCount = computed(() => seasonTargets(episodes.value, downloadStates.value).length)
  // offlineRuntimeReady() is non-reactive (navigator.serviceWorker.controller) —
  // a plain check in the template would never appear after the SW's first claim.
  // Track it in a ref, refreshed when the SW becomes ready.
  const canDownload = ref(false)
  // Downloads are app-only: in a plain browser tab every download surface is
  // hidden entirely (owner call 2026-07-14) — useDownloadGate owns that policy
  // for all surfaces.
  const { appOnly } = useDownloadGate()
  const downloadMode = computed<'off' | 'ready'>(() => {
    if (deps.getOffline() || !offlineDownloadsEnabled) return 'off'
    if (appOnly.value) return 'off'
    return canDownload.value ? 'ready' : 'off'
  })

  async function refreshDownloadStates() {
    if (!offlineRuntimeReady()) return
    const all = await listDownloads()
    const mine: Record<number, DownloadState> = {}
    for (const d of all) if (d.animeId === deps.getAnimeId()) mine[d.episode.number] = d.state
    downloadStates.value = mine
  }
  onMounted(() => {
    canDownload.value = offlineRuntimeReady()
    navigator.serviceWorker?.ready.then(() => { canDownload.value = offlineRuntimeReady() }).catch(() => {})
    void refreshDownloadStates()
  })
  // Throttle: engineState.progress ticks per segment (~300×/episode); a raw
  // watcher would hammer IndexedDB with listDownloads() on every tick.
  let dlRefreshQueued = false
  let dlRefreshTimer: ReturnType<typeof setTimeout> | null = null
  watch(engineState.progress, () => {
    if (dlRefreshQueued) return
    dlRefreshQueued = true
    dlRefreshTimer = setTimeout(() => { dlRefreshQueued = false; void refreshDownloadStates() }, 1000)
  })
  watch(() => engineState.cellularPauses.value, () => {
    // DownloadsPage mounts an inline offline AePlayer — skip there or the
    // page's own watcher (Task 10) double-toasts the same event.
    if (deps.getOffline()) return
    toast.push(t('player.aePlayer.offline.cellularAutoPaused'), 'info', 5000)
  })

  function clearDlRefreshTimer() {
    if (dlRefreshTimer) clearTimeout(dlRefreshTimer)
  }

  // Bundled entries come from the CURRENT stream (per-episode availability is
  // re-matched by the engine); external entries from the aggregated list.
  const dlSubOptions = computed<SubOption[]>(() => {
    const opts: SubOption[] = []
    const seenLangs = new Set<string>()
    const bundledUrls = new Set<string>()
    for (const tr of deps.getBundledTracks()) {
      bundledUrls.add(tr.url)
      if (seenLangs.has(tr.lang)) continue
      seenLangs.add(tr.lang)
      opts.push({ key: `b:${tr.lang}`, label: `${t('player.aePlayer.offline.subsBundled')} · ${tr.lang.toUpperCase()}`, pref: { kind: 'bundled', lang: tr.lang } })
    }
    opts.push(...externalSubOptions(deps.getSubtitleTracks().filter((tr) => !bundledUrls.has(tr.url))))
    return opts
  })

  function dlLoadTeams(provider: string, audio: AudioKind): Promise<string[]> {
    return resolver.listTeams(provider, deps.getAnimeId(), audio)
  }

  function onDownloadSeason() {
    downloadDialogOpen.value = true
    void deps.ensureSubsLoaded() // aggregated tracks feed the dialog's subtitle picker
  }

  async function onConfirmDownload(quality: string, combo: Combo | null, subPref: SubPref | null) {
    downloadDialogOpen.value = false
    const comboSnapshot = combo ? { ...combo } : { ...state.combo.value } // freeze — user may switch sources mid-download
    const resolveSubsFor = makeExternalSubResolver(deps.getAnimeId(), subPref)
    // A dialog-picked provider lists episodes with its own keys — re-list via
    // that provider before computing the season targets against it.
    let eps = episodes.value
    if (comboSnapshot.provider !== state.combo.value.provider) {
      try {
        eps = await resolver.listEpisodes(comboSnapshot.provider, deps.getAnimeId())
      } catch {
        toast.push(t('player.aePlayer.offline.sourceListFailed'), 'error')
        return
      }
    }
    const targets = seasonTargets(eps, downloadStates.value)
    const meta = deps.getAnimeMeta()
    await enqueueSeason(targets, {
      animeId: deps.getAnimeId(),
      animeTitle: meta.title,
      poster: meta.still,
      combo: comboSnapshot,
      quality,
      durationMin: meta.durationMin,
      subPref: subPref ?? undefined,
      resolveSubsFor,
      resolveFor: (target) => () => resolver.resolveStream(comboSnapshot.provider, deps.getAnimeId(), target, comboSnapshot),
    })
    void refreshDownloadStates()
  }

  return {
    downloadStates,
    seasonCount,
    downloadMode,
    dlSubOptions,
    dlLoadTeams,
    onDownloadSeason,
    onConfirmDownload,
    clearDlRefreshTimer,
  }
}
