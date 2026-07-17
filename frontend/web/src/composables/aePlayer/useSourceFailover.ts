import { ref, watch, type ComputedRef, type Ref } from 'vue'
import { pickSmartDefault } from '@/composables/aePlayer/smartDefault'
import { groupOfProvider } from '@/composables/aePlayer/useProviderFeed'
import { recordFallbackIntent, resetFallbackIntents } from '@/composables/aePlayer/sourceFallbackDebug'
import { formatEdgeTrail } from '@/composables/aePlayer/edgeTrail'
import { recordPlayerEvent } from '@/utils/playerTelemetry'
import { ladder, shouldDeferStallToLadder } from '@/utils/protocolLadder'
import {
  classifyPlaybackFailure,
  mapErrorKind,
  type FailureInputs,
} from '@/components/player/aePlayer/playbackFailure'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import type { useToast } from '@/composables/useToast'
import type { PlaybackStats } from '@/composables/aePlayer/usePlaybackStats'
import type { StreamResult, ProviderRow } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Dynamic-BEST source switching + terminal-failure telemetry ───────────────
// When an AUTO-selected source fails to actually play (a dead playlist at
// playback — e.g. a megaplay CDN host that 403/502s our IP), advance through
// candidate sources — untried servers of the current provider (megaplay hands
// a different CDN host per server) then the next-ranked providers — until one
// plays. "BEST" = the best source that actually works. Also owns the
// silent-stall watchdog and the always-on diagnostic bundle for the
// playback_failed telemetry event.

export interface SourceFailoverDeps {
  engine: ReturnType<typeof useVideoEngine>
  state: PlayerState
  videoRef: Ref<HTMLVideoElement | null>
  rows: ComputedRef<ProviderRow[]>
  report: ComputedRef<CapabilityReport | null>
  resolvedServers: Ref<{ id: string; label: string }[]>
  currentStream: Ref<StreamResult | null>
  selectedEpisode: Ref<EpisodeOption | null>
  sourceError: Ref<string | null>
  roomPinned: ComputedRef<boolean>
  getAnimeId: () => string
  /** Late-bound (clock/debug clusters come later in the composition). */
  getHasStarted: () => boolean
  getPlaybackBlocked: () => boolean
  getPlaybackStats: () => PlaybackStats | null | undefined
  /** Monotonic resolve-request token owned by the resolution cluster. */
  getResolveToken: () => number
  recordDecision: (reason: string) => void
  toast: ReturnType<typeof useToast>
  t: (key: string) => string
}

/** Watchdog progress signal that works on NATIVE playback (iPhone HLS /
 *  MP4 src), where hls.js counters and ladder xhr taps stay 0 forever:
 *  parsed metadata or any buffered range proves the source is alive. */
export function hasMediaArrived(v: HTMLVideoElement | null): boolean {
  if (!v) return false
  return v.readyState >= HTMLMediaElement.HAVE_METADATA || v.buffered.length > 0
}

export function useSourceFailover(deps: SourceFailoverDeps) {
  const { engine, state, videoRef, rows, report, resolvedServers, currentStream, selectedEpisode, sourceError, roomPinned, toast, t } = deps

  // props.animeId can change without a remount (no :key on the player), so the
  // per-title selection state must be reset when the title changes (the ae
  // library-presence check now lives backend-side, surfaced as state:'no_content').
  // Reset the saved-combo fallback so the new title gets a fresh attempt.
  // Reactive so the shareable-URL sync (urlSyncState) can distinguish a
  // user-pinned source from an auto/smart-default one.
  const providerAutoSelected = ref(false)

  const triedSources = new Set<string>()
  let sourceSwitchAttempts = 0
  const MAX_SOURCE_SWITCHES = 5
  // True from the moment an auto-failover commits the switch (setProvider/setServer)
  // until the next resolve takes over (isResolving flips true). During that handoff
  // the OLD <video> element can fire a native error for the source we're leaving —
  // onVideoError must NOT mistake that for a dead destination and strand the
  // recovering source behind "Stream unavailable".
  let switchingSource = false

  // Terminal playback-failure telemetry state. `attemptTrail` is the cross-source
  // failover history (the engine's edgeTrail only covers per-CDN edges within one
  // source); `emittedFailureKeys` de-dups so a retry streak on one broken episode
  // records at most one row per (tag, episode).
  const attemptTrail: Array<{ provider: string; server: string; reason: string }> = []
  const emittedFailureKeys = new Set<string>()

  function resetSourceSwitching() {
    triedSources.clear()
    sourceSwitchAttempts = 0
    switchingSource = false
    attemptTrail.length = 0
    emittedFailureKeys.clear()
  }

  /** The resolve path owns error handling once it starts — handoff window over. */
  function noteResolveStarted() {
    switchingSource = false
  }

  function isSwitchingSource(): boolean {
    return switchingSource
  }

  // Full diagnostic bundle ("all logs"), assembled regardless of hacker mode
  // (debugStats stays hacker-gated for the HUD; this is the always-on copy).
  function buildDiagnosticBundle() {
    const frs = engine.fragStats.value
    const last = frs[frs.length - 1]
    // Optional chaining: servedEdge/edgeTrail are only populated for
    // Kodik/solodcdn sources (the real useVideoEngine() always defines the
    // refs — empty string for non-Kodik sources); the `?.` here is purely a
    // safety net against a degenerate/partial engine object.
    const edge = engine.servedEdge?.value ?? ''
    const trail = engine.edgeTrail?.value ?? ''
    const playbackStats = deps.getPlaybackStats()
    return {
      combo: { ...state.combo.value },
      engine: {
        bw_bps: engine.bandwidthEstimate.value,
        level:
          engine.currentLevelLabel.value ||
          (currentStream.value?.type === 'mp4' ? 'mp4' : ''),
        frag_size_kb: last ? Math.round(last.size / 1024) : 0,
        frag_load_ms: last ? Math.round(last.loadMs) : 0,
        served_edge: edge,
        edge_trail: edge ? formatEdgeTrail(trail) : '',
        edge_rotations: edge && trail ? trail.split(',').length - 1 : 0,
        buffer_ahead_s: playbackStats?.bufferAheadSec ?? 0,
        buffer_behind_s: playbackStats?.bufferBehindSec ?? 0,
        video_ready_state: videoRef.value?.readyState ?? 0,
      },
      attempt_trail: attemptTrail.slice(-30),
      capability_snapshot: rows.value
        .slice(0, 40)
        .map((r) => ({ provider: r.id, group: r.group, state: r.state })),
      client: {
        ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
        viewport:
          typeof window !== 'undefined' ? `${window.innerWidth}x${window.innerHeight}` : '',
        connection:
          (typeof navigator !== 'undefined' &&
            (navigator as { connection?: { effectiveType?: string } }).connection
              ?.effectiveType) ||
          '',
      },
    }
  }

  // Classify the current failure; on a positive, de-duped decision, emit one
  // playback_failed telemetry event with the diagnostic bundle.
  function reportIfTerminal(inputs: FailureInputs) {
    const d = classifyPlaybackFailure(inputs)
    if (!d.emit || !d.tag) return
    const key = `${d.tag}:${deps.getAnimeId()}:${selectedEpisode.value?.number ?? ''}`
    if (emittedFailureKeys.has(key)) return
    emittedFailureKeys.add(key)
    recordPlayerEvent({
      kind: 'playback_failed',
      provider: inputs.failingProvider,
      anime_id: deps.getAnimeId(),
      episode: selectedEpisode.value?.number,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
      error_kind: mapErrorKind(inputs.reason),
      detail: {
        schema_version: 1,
        ...buildDiagnosticBundle(),
        reason: d.tag,
        all_exhausted: d.exhausted ?? false,
        is_first_party: inputs.firstParty,
        fail_reason: inputs.reason,
      },
    })
  }

  watch(() => deps.getAnimeId(), () => {
    providerAutoSelected.value = false
    resetSourceSwitching()
    resetFallbackIntents()
  })

  // Switch to the next candidate source after a playback failure. Returns true if
  // it actually initiated a switch (the combo/provider watcher re-resolves), false
  // otherwise. Only an AUTO-selected source is switched — a manual pick is the
  // user's deliberate choice and is left alone.
  //
  // HACKER MODE — do NOT auto-switch. The whole point of hacker mode is to verify
  // the smart-source behavior manually before trusting it to act, AND server-side
  // probing can't see what actually plays in the real user's browser (a CDN host
  // that 403s our datacenter may stream fine for a residential viewer). So in
  // hacker mode we record the fallback the resolver WOULD make (`acted: false`)
  // and stay put — letting you confirm whether the current source really plays —
  // instead of thrashing away from a source that may be working. With hacker mode
  // off the switch is performed and the same ledger records it with `acted: true`.
  async function advanceToNextSource(reason: string): Promise<boolean> {
    const failingProvider = state.combo.value.provider
    const server = state.combo.value.server || resolvedServers.value[0]?.id || ''
    const curKey = `${failingProvider}:${server}`

    // Compute the next candidate WITHOUT committing — hacker mode only records the
    // intent, and the failure classifier needs to know if any candidate remains.
    const triedWithCurrent = new Set(triedSources)
    triedWithCurrent.add(curKey)
    const nextServer = resolvedServers.value.find(
      (s) => !triedWithCurrent.has(`${failingProvider}:${s.id}`),
    )
    let toProvider: string | null = null
    let switchServerId: string | null = null
    if (nextServer) {
      toProvider = failingProvider
      switchServerId = nextServer.id
    } else {
      const triedProviders = new Set([...triedWithCurrent].map((k) => k.split(':')[0]))
      toProvider = pickSmartDefault(rows.value.filter((r) => !triedProviders.has(r.id)))?.id ?? null
    }
    const candidateExists = Boolean(switchServerId) || Boolean(toProvider)

    // Record the cross-source failover history, then decide (from pre-mutation
    // state) whether this is a terminal, alert-worthy playback failure.
    attemptTrail.push({ provider: failingProvider, server, reason })
    reportIfTerminal({
      reason,
      failingProvider,
      hackerMode: state.hackerMode.value,
      roomPinned: roomPinned.value,
      providerAutoSelected: providerAutoSelected.value,
      candidateExists,
      attemptsExceeded: sourceSwitchAttempts >= MAX_SOURCE_SWITCHES,
      // AUTO-608: group-derived, not a literal 'ae' id check, so a second
      // first-party provider trips the same alert. The `|| failingProvider ===
      // 'ae'` safety net is NOT redundant: report.value (useCapabilities) can
      // be null/stale mid-failure — props.animeId is reactive (no :key remount
      // on route param change, see Anime.vue), so navigating to a new anime
      // while the old stream is still erroring can race the capability refetch
      // through a null report (fetch-error branch) or a stale one that doesn't
      // list the old provider. resolveStreamForEpisode's `if (!provider) return`
      // guard rules this out for the FIRST resolve of a given combo — but not
      // for a later failure (silent stall / playback fatal) on an
      // already-resolved stream once the anime id has since changed underneath
      // it. Keeping the literal check preserves today's guarantee for 'ae'
      // itself in that window; only a genuinely-new first-party id would miss
      // the net there, and it'd still resolve correctly once the new report
      // loads.
      firstParty: groupOfProvider(report.value, failingProvider) === 'firstparty' || failingProvider === 'ae',
    })

    // ── Original control flow (unchanged) ──────────────────────────────────────
    // In a Watch-Together room the source is pinned to the shared room combo:
    // per-member auto-failover would diverge members onto different streams
    // (different encodes/intros → drift sync meaningless). Today this is also
    // covered because providerAutoSelected stays false in room mode, but guard
    // explicitly so a future change to that flag can't reintroduce divergence.
    if (roomPinned.value) return false
    if (!providerAutoSelected.value) return false
    if (sourceSwitchAttempts >= MAX_SOURCE_SWITCHES) return false

    const provider = failingProvider
    if (state.hackerMode.value) {
      recordFallbackIntent({ from: provider, to: toProvider, reason, acted: false })
      const target = switchServerId ? `${provider} · server ${switchServerId}` : toProvider
      sourceError.value = target
        ? `Hacker mode: auto-switch suppressed — would try ${target} (${reason}). Pick a source manually.`
        : `Hacker mode: auto-switch suppressed — ${provider} failed (${reason}), no fallback candidate.`
      return false
    }

    if (!toProvider) return false

    // Commit the switch.
    triedSources.add(curKey)
    sourceSwitchAttempts++
    switchingSource = true // suppress the outgoing element's error during handoff
    if (switchServerId) {
      state.setServer(switchServerId) // combo watcher re-resolves the stream
    } else {
      providerAutoSelected.value = true
      state.setProvider(toProvider, '') // provider watcher re-lists + re-resolves
    }
    recordFallbackIntent({ from: provider, to: toProvider, reason, acted: true })
    deps.recordDecision(
      switchServerId
        ? `auto-failover — switched server (${reason})`
        : `auto-failover — previous source failed (${reason})`,
    )
    return true
  }

  // Silent-stall watchdog. A CODECS-less HLS manifest (megaplay/owocdn) loads fine
  // but hls.js then requests ZERO fragments and emits NO error — the player just
  // hangs at 0:00 (the documented platform-wide codec stall). The fatal-driven
  // switch can't see that. So after a stream attaches, if NO fragment has loaded
  // AND playback never started within the window, treat it as a dead source and
  // advance. Guarded on fragLoadedCount === 0 so a merely-slow stream (fragments
  // trickling in) is NOT switched away from.
  let playbackWatchdog: ReturnType<typeof setTimeout> | null = null
  const PLAYBACK_WATCHDOG_MS = 12000
  function clearPlaybackWatchdog() {
    if (playbackWatchdog) {
      clearTimeout(playbackWatchdog)
      playbackWatchdog = null
    }
  }
  function armPlaybackWatchdog() {
    clearPlaybackWatchdog()
    const tok = deps.getResolveToken()
    playbackWatchdog = setTimeout(() => {
      if (tok !== deps.getResolveToken()) return      // superseded by a newer resolve
      if (deps.getHasStarted()) return                // already playing
      if (sourceError.value) return                   // already errored/handled
      // Browser vetoed play() (NotAllowedError): the stream is fine, only the
      // START was blocked. Without this guard the watchdog misreads "blocked" as
      // "stalled" — especially on native MP4 sources where fragLoadedCount stays
      // 0 — and churns through every provider pulling gigabytes for nothing.
      if (deps.getPlaybackBlocked()) return
      // A first fragment that is downloading (bytes flowing) is SLOW, not dead —
      // aborting it re-resolves the same source forever (the 2026-07-11 tNeymik
      // "stale" loop: seg0 restarted 3×, video never possible). Let the ladder's
      // projected-too-slow rule downshift the tier instead; just re-arm.
      if (shouldDeferStallToLadder(ladder.inflight())) {
        armPlaybackWatchdog()
        return
      }
      if (engine.fragLoadedCount.value > 0) return // fragments flowing — just slow
      // Native playback (iPhone HLS via AVFoundation, plain MP4 src) never
      // touches hls.js counters or the ladder's xhr taps — both stay 0 for a
      // perfectly healthy stream. The element itself is the only witness
      // there: metadata parsed or a buffered range = media arriving, NOT a
      // dead source. Without this the watchdog churned every provider to
      // all_exhausted while segments were flowing (2026-07-16 iPhone report).
      if (hasMediaArrived(videoRef.value)) return
      void (async () => {
        if (await advanceToNextSource('silent stall')) {
          toast.push(t('player.aePlayer.switchNotPlaying'), 'info', 4000)
        } else if (!sourceError.value) {
          sourceError.value = t('player.aePlayer.streamUnavailable')
        }
      })()
    }, PLAYBACK_WATCHDOG_MS)
  }

  return {
    providerAutoSelected,
    resetSourceSwitching,
    noteResolveStarted,
    isSwitchingSource,
    advanceToNextSource,
    armPlaybackWatchdog,
    clearPlaybackWatchdog,
  }
}
