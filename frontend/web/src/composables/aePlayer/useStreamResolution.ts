import { watch, type ComputedRef, type Ref } from 'vue'
import { pickEpisodeForProvider, providerMissesTargetEpisode, shouldReselectEpisode } from '@/composables/aePlayer/episodeSelection'
import { recordPlayerEvent } from '@/utils/playerTelemetry'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { ProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import type { useWatchTracking } from '@/composables/aePlayer/useWatchTracking'
import type { useToast } from '@/composables/useToast'
import type { StreamResult, ProviderRow, ServerOption } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { MenuKind } from '@/composables/aePlayer/usePlayerMenus'

// ── Episode list + stream resolution ─────────────────────────────────────────
// The resolve heart of the player: lists episodes/teams for the selected
// provider, resolves the stream for the selected episode, and owns the
// provider/facet/episode watchers that trigger those resolutions. Auto-failover
// on failure is delegated to the failover cluster.

/** Monotonically-increasing request token — only the latest resolve applies.
 *  Prevents a stale audio/lang/server re-resolve from clobbering a concurrent
 *  provider-change full re-list+resolve that started after it. Shared with the
 *  failover cluster's stall watchdog (component-owned box). */
export interface ResolveEpoch {
  token: number
}

/** Best-effort telemetry timing state; never influences playback logic.
 *  Component-owned so the buffering/first-frame glue can read/write it too. */
export interface ResolveTelemetryTiming {
  resolveStartedAt: number
  reachedReported: boolean
  stallStartedAt: number
}

export interface StreamResolutionDeps {
  state: PlayerState
  resolver: ProviderResolver
  engine: ReturnType<typeof useVideoEngine>
  tracking: ReturnType<typeof useWatchTracking>
  toast: ReturnType<typeof useToast>
  t: (key: string) => string
  recordDecision: (reason: string) => void
  getAnimeId: () => string
  getInitialEpisode: () => number | undefined
  // Shared reactive state (component-owned)
  episodes: Ref<EpisodeOption[]>
  selectedEpisode: Ref<EpisodeOption | null>
  userPickedEpisode: Ref<boolean>
  resolvedServers: Ref<ServerOption[]>
  teams: Ref<string[]>
  currentStream: Ref<StreamResult | null>
  isResolving: Ref<boolean>
  sourceError: Ref<string | null>
  openMenu: Ref<MenuKind>
  rows: ComputedRef<ProviderRow[]>
  resolveEpoch: ResolveEpoch
  telemetryTiming: ResolveTelemetryTiming
  // Failover cluster
  providerAutoSelected: Ref<boolean>
  roomPinned: ComputedRef<boolean>
  advanceToNextSource: (reason: string) => Promise<boolean>
  armPlaybackWatchdog: () => void
  resetSourceSwitching: () => void
  noteResolveStarted: () => void
  // Playback clock cluster
  hasStarted: Ref<boolean>
  resetPlaybackClock: () => void
  capturePlayhead: (keep: boolean) => { restoreT: number; wasPlaying: boolean }
  restorePlayhead: (t: number, wasPlaying: boolean) => void
  // Sibling clusters (late-bound where they compose after this one)
  resetPlaybackBlocked: () => void
  applyInitialSeek: () => void
  applyOfflineAutoSub: (epNumber: number, stream: StreamResult) => void
  isEpisodeWatched: (n: number) => boolean
  broadcastEpisodeChange: (ep: EpisodeOption) => void
  pickFacetDefault: () => ProviderRow | null
  resumeChipDismissed: Ref<boolean>
  resumeChipUsed: Ref<boolean>
}

export function useStreamResolution(deps: StreamResolutionDeps) {
  const {
    state, resolver, engine, tracking, toast, t,
    episodes, selectedEpisode, userPickedEpisode, resolvedServers, teams,
    currentStream, isResolving, sourceError, openMenu, rows,
    resolveEpoch, telemetryTiming, providerAutoSelected, roomPinned,
    hasStarted, resumeChipDismissed, resumeChipUsed,
  } = deps

  // Latest-wins guard for the team chips, independent of resolveToken: the team
  // list is (re)loaded both on provider switch and on a same-provider audio
  // toggle, so it needs its own epoch so a slow earlier fetch can't overwrite a
  // newer one.
  let teamsToken = 0

  // Initialize selectedEpisode from initialEpisode (NEVER episodesAired).
  function initSelectedEpisode() {
    const targetEp = deps.getInitialEpisode() ?? 1
    const found = episodes.value.find((e) => e.number === targetEp)
    if (found) {
      selectedEpisode.value = found
    } else if (episodes.value.length > 0) {
      selectedEpisode.value = episodes.value[0]
    } else {
      // Synthetic placeholder for when the episode list isn't loaded yet.
      selectedEpisode.value = { key: targetEp, label: targetEp, number: targetEp }
    }
  }

  // Resume / watch-progress resolves asynchronously AFTER mount, so initialEpisode
  // flips from its default (1) to e.g. lastWatched+1 a tick later. Re-pick then —
  // unless the user already chose an episode. Closes the mount race that used to
  // fall back to episodesAired and open the latest aired episode instead of ep 1.
  watch(
    () => deps.getInitialEpisode(),
    () => {
      if (shouldReselectEpisode(selectedEpisode.value?.number ?? null, deps.getInitialEpisode(), userPickedEpisode.value)) {
        initSelectedEpisode()
      }
    },
  )

  // Load the provider-native team chips (e.g. Kodik translation titles) for the
  // CURRENT audio facet. Kodik exposes different teams for sub vs dub, so the list
  // is scoped to state.combo.audio — otherwise SUB shows a wall of DUB teams.
  // Best-effort and self-epoched (teamsToken) so it never blocks or races a
  // stream resolve.
  function loadTeams(provider: string) {
    const tok = ++teamsToken
    teams.value = [] // clear stale chips immediately
    resolver
      .listTeams(provider, deps.getAnimeId(), state.combo.value.audio)
      .then((t) => { if (tok === teamsToken) teams.value = t })
      .catch(() => { if (tok === teamsToken) teams.value = [] })
  }

  // Reload the team chips when the audio facet flips on the SAME provider (a
  // sub↔dub toggle on Kodik). Provider switches reload teams via
  // loadEpisodesAndStream; this covers the in-place toggle so the chips track the
  // selected audio. Guarded on a real provider being selected.
  watch(() => state.combo.value.audio, () => {
    const provider = state.combo.value.provider
    if (provider) loadTeams(provider)
  })

  async function loadEpisodesAndStream() {
    const provider = state.combo.value.provider
    if (!provider) return

    // Playhead-preservation: capture the episode the viewer is on BEFORE the
    // re-list (which may reassign selectedEpisode). If the switched-to source
    // still serves this episode, the playhead is restored after load (below) so a
    // mid-watch source switch doesn't restart at 0:00 or regress saved progress.
    const keepEpNum = selectedEpisode.value?.number ?? null

    sourceError.value = null
    isResolving.value = true
    deps.noteResolveStarted() // resolve owns error handling now — handoff window over
    hasStarted.value = false
    deps.resetPlaybackBlocked() // new stream = a fresh autoplay verdict
    // Re-listing a provider: its servers are unknown until the resolve below, so
    // drop the previous provider's stale set. This also keeps advanceToNextSource's
    // server-first dodge from targeting a nonexistent server of the new provider.
    resolvedServers.value = []
    const token = ++resolveEpoch.token
    telemetryTiming.resolveStartedAt = performance.now()
    telemetryTiming.reachedReported = false
    telemetryTiming.stallStartedAt = 0

    try {
      // Load episode list
      const eps = await resolver.listEpisodes(provider, deps.getAnimeId())
      if (token !== resolveEpoch.token) return // superseded by a later request

      episodes.value = eps

      // Which episode does the viewer want? Resume-resolved selection, else the
      // initial/first episode.
      const targetNum =
        selectedEpisode.value?.number ?? deps.getInitialEpisode() ?? 1

      // Episode-aware auto-default: a source that was AUTO-selected (smart default
      // or failover) but doesn't actually carry the episode the viewer wants is the
      // wrong source for them — ae's partial library can hold only ep 27 while a
      // first-time viewer wants ep 1, and pickEpisodeForProvider would otherwise
      // snap them UP to ep 27. Advance to the next-best source instead of silently
      // playing a different episode. A manual pick (providerAutoSelected=false) is
      // respected as-is; an exhausted / hacker-suppressed / room-pinned advance
      // falls through and plays what this source does have rather than dead-ending.
      // Runs BEFORE loadTeams so a source we're about to abandon never fires a
      // wasted teams fetch.
      if (providerAutoSelected.value && providerMissesTargetEpisode(eps, targetNum)) {
        if (await deps.advanceToNextSource('source missing the requested episode')) return
      }

      // Provider-native teams (e.g. Kodik translation titles) for the Source
      // panel, scoped to the current audio facet. Best-effort — never blocks the
      // stream resolve.
      loadTeams(provider)

      // Preserve the selected episode across provider changes: keep the same
      // episode NUMBER when the new source has it, and never snap back to EP 1
      // when it doesn't (pickEpisodeForProvider handles the nearest-fallback).
      const ep = pickEpisodeForProvider(eps, targetNum, selectedEpisode.value)

      if (!ep) {
        sourceError.value = t('player.aePlayer.noEpisodes')
        return
      }

      selectedEpisode.value = ep

      // Resolve stream
      const stream = await resolver.resolveStream(
        provider,
        deps.getAnimeId(),
        ep,
        state.combo.value,
      )

      if (token !== resolveEpoch.token) return // superseded

      resolvedServers.value = stream.servers ?? []
      // Set BEFORE the await: a superseded resolve must never clobber the
      // winner's stream descriptor after resuming from engine.load.
      currentStream.value = stream
      deps.applyOfflineAutoSub(ep.number, stream)
      // Restore the playhead only when the switched-to source stayed on the same
      // episode — a provider change that lands on a different episode starts fresh.
      const { restoreT, wasPlaying } = deps.capturePlayhead(keepEpNum !== null && ep.number === keepEpNum)
      deps.resetPlaybackClock() // drop the outgoing source's playhead before the swap
      await engine.load(stream)
      deps.applyInitialSeek() // shared-link `?t=` one-shot seek (no-op after first load)
      deps.restorePlayhead(restoreT, wasPlaying)
      deps.armPlaybackWatchdog() // catch a silent CODECS-less stall (manifest OK, no frags)
    } catch (err: unknown) {
      if (token !== resolveEpoch.token) return // superseded
      const isNotAvailable =
        err instanceof Error && err.name === 'NotAvailableError'
      // Telemetry: resolve failure (best-effort, never throws)
      recordPlayerEvent({
        kind: 'resolve',
        provider: state.combo.value.provider,
        anime_id: deps.getAnimeId(),
        episode: selectedEpisode.value?.number,
        outcome: 'fail',
        reached_playback: false,
        error_kind: isNotAvailable ? 'not_available' : 'stream_error',
        latency_ms: telemetryTiming.resolveStartedAt ? Math.round(performance.now() - telemetryTiming.resolveStartedAt) : undefined,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
      // Any resolve failure (not-available OR HTTP/stream error like allanime's
      // 500) advances the dynamic-BEST chain to the next candidate, so it keeps
      // going until a source actually resolves AND plays — never strands on a
      // dead provider.
      if (await deps.advanceToNextSource('resolve failed')) {
        toast.push(t('player.aePlayer.switchNotAvailable'), 'info', 4000)
        return
      }
      // advanceToNextSource may have set a hacker-mode "suppressed" message — keep it.
      if (!sourceError.value) {
        sourceError.value = isNotAvailable ? t('player.aePlayer.sourceUnavailable') : t('player.aePlayer.streamUnavailable')
      }
    } finally {
      if (token === resolveEpoch.token) {
        isResolving.value = false
      }
    }
  }

  // Provider change → full re-list + resolve (episodes are provider-specific)
  watch(
    () => state.combo.value.provider,
    (newProvider) => {
      if (newProvider) {
        void loadEpisodesAndStream()
      }
    },
  )

  // Audio / language / server / team change, or episode selection → re-resolve stream
  // only (no need to re-list episodes). The token inside resolveStreamForEpisode
  // ensures a concurrent provider-change full-resolve wins if they race.
  // Skip when loadEpisodesAndStream is in-flight: it sets selectedEpisode itself
  // and will call resolveStream at the end — we must not fire a duplicate.
  watch(
    () => [
      state.combo.value.audio,
      state.combo.value.lang,
      state.combo.value.server,
      state.combo.value.team,
      selectedEpisode.value,
    ] as const,
    (newVal, oldVal) => {
      // Skip the very first run (oldVal is undefined on initial watch fire)
      if (oldVal === undefined) return
      // Skip if a full re-list is already in progress (provider changed)
      if (isResolving.value) return
      // Skip an EPISODE change while the episode list hasn't loaded: that's the
      // mount-time null→synthetic-placeholder transition from
      // initSelectedEpisode, whose key is a bare episode number — scraper
      // episode ids are opaque, so resolving it fires a doomed
      // scraper/servers?episode=<number> request (seen in prod HARs on
      // ?episode=N deep links). loadEpisodesAndStream reconciles the selection
      // and resolves the stream itself once the real list arrives. Combo
      // (audio/lang/server) changes are NOT gated on the list.
      if (newVal[4] !== oldVal[4] && episodes.value.length === 0) return
      // An AUDIO or LANGUAGE change is a facet change: the current provider may no
      // longer serve it (e.g. switching to English while on Kodik, which is
      // RU-only). When it can't, drop the current stream and re-pick the best
      // provider for the new facet — refreshing episodes/servers/teams — instead
      // of silently re-resolving a provider that has no such stream.
      const audioChanged = newVal[0] !== oldVal[0]
      const langChanged = newVal[1] !== oldVal[1]
      // Under RAW the language slider is hidden — combo.lang is a DERIVED value that
      // follows the chosen provider (see the lang-follows-provider watcher in the
      // combo-bootstrap cluster). A RAW-only lang change must NOT repick or
      // re-resolve (it would churn the source); language is a real facet only
      // under DUB.
      if (!audioChanged && langChanged && newVal[0] === 'sub') return
      const facetChanged = audioChanged || (newVal[0] === 'dub' && langChanged)
      if (facetChanged && !roomPinned.value) {
        void repickProviderForFacet()
        return
      }
      void resolveStreamForCurrentEpisode()
    },
  )

  // Re-pick the provider after an audio/language change. If the current provider
  // still serves the new facet, keep it and just re-resolve (so a sub↔dub toggle
  // on a provider that has both stays put). Otherwise drop the now-wrong stream
  // and switch to the top-ranked provider that serves the new facet, which
  // triggers a full re-list + team/server refresh via the provider watcher.
  function repickProviderForFacet() {
    // rows is a computed over the live audio/lang filter — already reflects the
    // just-changed facet, no manual recompute needed.
    const cur = state.combo.value.provider
    const curStillActive = rows.value.some((r) => r.id === cur && r.state === 'active')
    if (curStillActive) {
      deps.recordDecision('kept current source — it still serves your audio / language')
      void resolveStreamForCurrentEpisode()
      return
    }
    // Current provider can't serve the new facet — drop its stream immediately so
    // the user doesn't keep hearing the wrong-language audio while we resolve.
    engine.destroy()
    sourceError.value = null
    deps.resetSourceSwitching()
    const pick = deps.pickFacetDefault()
    if (!pick) {
      sourceError.value = t('player.aePlayer.noSourceForFacet')
      return
    }
    providerAutoSelected.value = true
    state.setProvider(pick.id, '') // provider watcher re-lists episodes + refreshes teams
    deps.recordDecision('re-picked best source for the new audio / language')
  }

  // ─── Provider selection helper ──────────────────────────────────────────────

  function onSelectProvider(id: string) {
    providerAutoSelected.value = false
    deps.resetSourceSwitching() // manual pick — fresh state, and don't auto-switch it
    state.setProvider(id, '')
    deps.recordDecision('manual — you picked this source')
    // loadEpisodesAndStream fires via the provider watcher above
  }

  // ─── Episode selection (episodes drawer) ────────────────────────────────────
  // Resolve DIRECTLY (mirrors goToNextEpisode) — the combo/episode watcher
  // early-returns while isResolving and would silently swallow a click made
  // during an in-flight resolve. resolveStreamForEpisode sets isResolving
  // synchronously, so the watcher's deferred fire is deduped, and resolveToken
  // arbitrates any race with the in-flight request.

  function onSelectEpisode(ep: EpisodeOption) {
    openMenu.value = null
    if (selectedEpisode.value?.number === ep.number) return
    // WT: broadcast a genuine local episode pick to the room (echo-guarded in
    // the room-sync cluster).
    deps.broadcastEpisodeChange(ep)
    deps.resetSourceSwitching() // new episode — fresh switch budget
    tracking.saveNow() // persist the outgoing episode's position
    selectedEpisode.value = ep
    userPickedEpisode.value = true
    tracking.resetEpisode(deps.isEpisodeWatched(ep.number))
    resumeChipDismissed.value = false
    resumeChipUsed.value = false
    void resolveStreamForEpisode(ep)
  }

  // ─── Retry ──────────────────────────────────────────────────────────────────

  function retryResolution() {
    sourceError.value = null
    deps.resetSourceSwitching() // manual retry — give every candidate a fresh shot
    void loadEpisodesAndStream()
  }

  async function resolveStreamForEpisode(ep: EpisodeOption, keepPosition = false) {
    const provider = state.combo.value.provider
    if (!provider) return
    sourceError.value = null
    isResolving.value = true
    deps.noteResolveStarted() // resolve owns error handling now — handoff window over
    hasStarted.value = false
    deps.resetPlaybackBlocked() // new stream = a fresh autoplay verdict
    const token = ++resolveEpoch.token
    telemetryTiming.resolveStartedAt = performance.now()
    telemetryTiming.reachedReported = false
    telemetryTiming.stallStartedAt = 0
    try {
      const stream = await resolver.resolveStream(
        provider,
        deps.getAnimeId(),
        ep,
        state.combo.value,
      )
      if (token !== resolveEpoch.token) return // superseded
      resolvedServers.value = stream.servers ?? []
      // Set BEFORE the await — see loadEpisodesAndStream.
      currentStream.value = stream
      deps.applyOfflineAutoSub(ep.number, stream)
      // Same-episode re-resolve (facet/server/team/quality switch) keeps the
      // viewer's spot; a genuine episode change (keepPosition=false) starts fresh.
      const { restoreT, wasPlaying } = deps.capturePlayhead(keepPosition)
      deps.resetPlaybackClock() // drop the outgoing source's playhead before the swap
      await engine.load(stream)
      deps.applyInitialSeek() // shared-link `?t=` one-shot seek (no-op after first load)
      deps.restorePlayhead(restoreT, wasPlaying)
      deps.armPlaybackWatchdog() // catch a silent CODECS-less stall (manifest OK, no frags)
    } catch (err: unknown) {
      if (token !== resolveEpoch.token) return // superseded
      const isNotAvailable =
        err instanceof Error && err.name === 'NotAvailableError'
      // Telemetry: resolve failure (best-effort, never throws)
      recordPlayerEvent({
        kind: 'resolve',
        provider: state.combo.value.provider,
        anime_id: deps.getAnimeId(),
        episode: ep.number,
        outcome: 'fail',
        reached_playback: false,
        error_kind: isNotAvailable ? 'not_available' : 'stream_error',
        latency_ms: telemetryTiming.resolveStartedAt ? Math.round(performance.now() - telemetryTiming.resolveStartedAt) : undefined,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
      if (await deps.advanceToNextSource('resolve failed')) {
        toast.push(t('player.aePlayer.switchNotAvailable'), 'info', 4000)
        return
      }
      if (!sourceError.value) {
        sourceError.value = isNotAvailable
          ? t('player.aePlayer.sourceUnavailable')
          : t('player.aePlayer.streamUnavailable')
      }
    } finally {
      if (token === resolveEpoch.token) {
        isResolving.value = false
      }
    }
  }

  // Re-resolve stream (without re-listing episodes) for the currently-selected
  // episode when audio, lang, server, or selectedEpisode changes.
  // Provider changes are handled separately by the provider watcher above
  // (which does a full re-list + resolve) — we skip here when it's active.
  async function resolveStreamForCurrentEpisode() {
    const ep = selectedEpisode.value
    if (!ep) return
    // Same episode, different stream (audio/lang/server/team/quality) → keep the
    // playhead so the swap is seamless and can't regress saved progress.
    await resolveStreamForEpisode(ep, true)
  }

  return {
    initSelectedEpisode,
    resolveStreamForEpisode,
    resolveStreamForCurrentEpisode,
    onSelectProvider,
    onSelectEpisode,
    retryResolution,
  }
}
