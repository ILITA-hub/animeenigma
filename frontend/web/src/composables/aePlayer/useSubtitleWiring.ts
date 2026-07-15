import { computed, watch, ref, type ComputedRef, type Ref } from 'vue'
import { useSubtitleTracks } from '@/composables/aePlayer/useSubtitleTracks'
import { pickBestForLang } from '@/composables/aePlayer/pickDefaultSubtitle'
import { useSubtitleCues } from '@/composables/aePlayer/useSubtitleCues'
import { useSubtitleAutoSyncPref } from '@/composables/aePlayer/useSubtitleAutoSyncPref'
import { useSubtitleAutoSync } from '@/composables/aePlayer/useSubtitleAutoSync'
import { pickOfflineAutoSub, type OfflinePlayback } from '@/offline/offlineAdapter'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useToast } from '@/composables/useToast'
import type { StreamResult, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Subtitles — OFF by default, on purpose ───────────────────────────────────
// Track aggregation (provider-bundled + Jimaku/OpenSubtitles), the chosen-track
// binding, VAD auto-sync wiring, and the subtitle quick-menu derivations.
// Subtitles default OFF (state.subLang starts 'off') and the player NEVER
// auto-enables one — the user opts in via the Subtitles menu. That choice
// (language + on/off) then PERSISTS across episodes: the track URL is
// episode-specific so it's dropped on episode change, but the re-bind watcher
// re-resolves a track in the chosen language for the new episode.

/** A selectable subtitle track — the aggregated SubtitleTrack contract
 *  (url/provider/lang/label/format); alias rather than redeclare. */
export type SubTrack = SubtitleTrack

export interface SubtitleWiringDeps {
  animeIdRef: Ref<string>
  getAnimeId: () => string
  getInitialEpisode: () => number | undefined
  getOffline: () => OfflinePlayback | null | undefined
  state: PlayerState
  videoRef: Ref<HTMLVideoElement | null>
  currentStream: Ref<StreamResult | null>
  selectedEpisode: Ref<EpisodeOption | null>
  activeProviderName: ComputedRef<string>
  browseOpen: Ref<boolean>
  toast: ReturnType<typeof useToast>
  t: (key: string, params?: Record<string, unknown>) => string
}

export function useSubtitleWiring(deps: SubtitleWiringDeps) {
  const { state, videoRef, currentStream, selectedEpisode, activeProviderName, browseOpen, toast, t } = deps

  const chosenSub = ref<SubTrack | null>(null)

  const chosenSubUrl = computed<string | null>(() => chosenSub.value?.url ?? null)
  // Whether a subtitle overlay is actually rendering — drives the CC button's
  // enabled affordance (distinct from the menu-open highlight).
  const subsOn = computed(() => state.subLang.value !== 'off' && !!chosenSubUrl.value)
  const chosenSubFormat = computed<'ass' | 'srt' | 'vtt' | null>(() => {
    const fmt = chosenSub.value?.format ?? null
    if (fmt === 'ass' || fmt === 'srt' || fmt === 'vtt') return fmt
    return null
  })

  // Episode number the subtitle aggregation keys on.
  const subEpisode = computed(() => selectedEpisode.value?.number ?? deps.getInitialEpisode() ?? 1)
  // Provider's own signed soft-subs from the resolved stream.
  const providerSubtitles = computed(() => currentStream.value?.subtitles)
  const {
    tracks: subtitleTracks,
    loading: subsLoading,
    error: subsError,
    providersDown: subsProvidersDown,
    ensureLoaded: ensureSubsLoaded,
    refetch: refetchSubs,
  } = useSubtitleTracks(deps.animeIdRef, subEpisode, providerSubtitles)

  // ─── Subtitle auto-sync (frontend VAD; spec 2026-06-29) ────────────────────
  const { cues: subtitleCues } = useSubtitleCues(chosenSubUrl, chosenSubFormat)
  const autoSyncEpisodeKey = computed(() => `${deps.getAnimeId()}:${subEpisode.value}`)
  const autoSyncPref = useSubtitleAutoSyncPref(autoSyncEpisodeKey)
  const autoSync = useSubtitleAutoSync({
    videoElement: videoRef,
    cues: subtitleCues,
    enabled: computed(() => autoSyncPref.enabled.value && subsOn.value),
    episodeKey: autoSyncEpisodeKey,
  })
  // Manual offset layers on top of the auto result.
  const effectiveOffset = computed(() => autoSync.autoOffset.value + state.subOffset.value)
  const autoSyncInfo = computed(() =>
    state.hackerMode.value
      ? { status: autoSync.status.value, offset: autoSync.autoOffset.value, confidence: autoSync.confidence.value, events: autoSync.syncEvents.value }
      : null,
  )

  // A NEW episode drops the stale (episode-specific) track URL but KEEPS the
  // user's subtitle language choice (state.subLang). Keyed on episode, NOT
  // currentStream: a same-episode re-resolve (server fallback, quality swap) must
  // not drop a track the user is watching.
  watch(subEpisode, () => {
    chosenSub.value = null
  })

  // Fetch the aggregation eagerly once a RAW (sub) stream resolves so the menu's
  // language list is ready — but DO NOT auto-enable (subs default off).
  watch(
    [currentStream, () => state.combo.value.audio],
    async () => {
      if (!currentStream.value || state.combo.value.audio !== 'sub') return
      await ensureSubsLoaded()
    },
  )

  // Re-bind the chosen track to the persisted subtitle language whenever the track
  // list changes (new episode, or late provider/aggregation arrival). 'off' stays
  // off — there is no auto-enable.
  watch(subtitleTracks, () => {
    const lang = state.subLang.value
    if (lang === 'off') return
    const track = pickBestForLang(subtitleTracks.value, lang)
    if (track) chosenSub.value = track
  })

  // Real distinct languages that have a loaded soft track (provider-bundled +
  // aggregated Jimaku/OpenSubtitles). Drives which RU/EN/JP fast buttons are enabled.
  const availableSubLangs = computed(() =>
    [...new Set(subtitleTracks.value.map((t) => t.lang))],
  )

  // Provider-bundled soft subs that shipped with the resolved stream.
  const providerBundledTracks = computed<SubTrack[]>(
    () => (providerSubtitles.value ?? []) as SubTrack[],
  )

  // Per-language source label for the quick menu rows ("Русский · Crunchyroll").
  // The best track for a language (bundled-first) supplies the meta line.
  const langSources = computed<Record<string, string>>(() => {
    const m: Record<string, string> = {}
    for (const l of ['ru', 'en', 'ja']) {
      const best = pickBestForLang(subtitleTracks.value, l)
      if (best) m[l] = best.label
    }
    return m
  })

  // Informational note for the subs menu: when the provider shipped no soft track
  // for an EN/RU SUB cut, the subs the user sees are hardsubbed into the video.
  // A raw original-JP cut (lang 'ja') is NOT hardsubbed — its subs come from the
  // optional Jimaku/OpenSubtitles overlay — so the note never applies there.
  const hardsubNote = computed(() => {
    if (chosenSub.value) return null
    if (state.combo.value.audio !== 'sub') return null
    if (state.combo.value.lang === 'ja') return null         // raw JP → overlay, not burned in
    if (providerBundledTracks.value.length > 0) return null  // provider soft subs → not hardsubbed
    const prov = activeProviderName.value
    if (!prov) return null
    return t('player.aePlayer.subs.hardsub', { provider: prov })
  })

  // Session opt-out: once the viewer explicitly turns subs off, offline
  // auto-enable must not re-arm on the next episode.
  let userDisabledSubs = false

  // The ONLY sanctioned subtitle auto-enable: explicit download-time choice,
  // offline playback only. Called after EVERY currentStream assignment.
  // Note: the "re-bind chosen track to subLang" watcher may later swap to
  // pickBestForLang's pick for the same lang — same track in practice.
  function applyOfflineAutoSub(epNumber: number, stream: StreamResult): void {
    const offline = deps.getOffline()
    if (!offline || userDisabledSubs) return
    const auto = pickOfflineAutoSub(offline, epNumber, stream.subtitles)
    if (auto) {
      chosenSub.value = auto as SubTrack
      state.subLang.value = auto.lang // session ref — the global "subs off by default" pref is untouched
    }
  }

  function onSelectSubTrack(track: SubTrack) {
    chosenSub.value = track
    // Selecting a track turns the overlay on for that language (persists across episodes).
    state.subLang.value = track.lang
    browseOpen.value = false
  }

  function onSubtitlesOff() {
    userDisabledSubs = true
    chosenSub.value = null
    state.subLang.value = 'off'
    browseOpen.value = false
  }

  // Quick-chooser RU/EN/JP language row → pick the best track for that language.
  function onPickSubLang(v: string) {
    if (v === 'off') { onSubtitlesOff(); return }
    const track = pickBestForLang(subtitleTracks.value, v)
    if (track) onSelectSubTrack(track)
  }

  // SubtitleOverlay failed to fetch/parse the chosen track (e.g. a dead upstream
  // link). Turn the selection off rather than leaving it silently stuck — the
  // video keeps playing, but the subtitle button should reflect reality so the
  // user knows to pick a different track from the Subtitles menu.
  function onSubtitleError() {
    toast.push(t('player.aePlayer.subtitleLoadFailed'), 'error', 5000)
    state.subLang.value = 'off'
  }

  return {
    chosenSubUrl,
    chosenSubFormat,
    subsOn,
    subtitleTracks,
    subsLoading,
    subsError,
    subsProvidersDown,
    ensureSubsLoaded,
    refetchSubs,
    effectiveOffset,
    autoSyncPref,
    autoSyncInfo,
    availableSubLangs,
    providerBundledTracks,
    langSources,
    hardsubNote,
    applyOfflineAutoSub,
    onSelectSubTrack,
    onSubtitlesOff,
    onPickSubLang,
    onSubtitleError,
  }
}
