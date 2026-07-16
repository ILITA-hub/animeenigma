import { computed, ref, watch, type Ref } from 'vue'
import { usePlaybackStats } from '@/composables/aePlayer/usePlaybackStats'
import { scrubDebug } from '@/composables/aePlayer/scrubPreviewDebug'
import { formatEdgeTrail } from '@/composables/aePlayer/edgeTrail'
import { ladder, formatLadderRows } from '@/utils/protocolLadder'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import type { SeekTrace, StreamResult } from '@/types/aePlayer'

// ── Hacker mode (debug HUD) ───────────────────────────────────────────────────
// Per-fragment playback stats, the seek pipeline trace, HUD visibility
// lifecycle (linger + fade), the scrub-bar fragment heatmap, and the compact
// debugStats line set for the settings menu.

export interface DebugToolsDeps {
  state: PlayerState
  engine: ReturnType<typeof useVideoEngine>
  videoRef: Ref<HTMLVideoElement | null>
  currentStream: Ref<StreamResult | null>
  duration: Ref<number>
  showBuffering: Ref<boolean>
  /** Live connection-health datum for the HUD ('ok' | 'slow' | 'offline'). */
  getConnectionState?: () => 'ok' | 'slow' | 'offline'
}

export function useDebugTools(deps: DebugToolsDeps) {
  const { state, engine, videoRef, currentStream, duration, showBuffering } = deps

  const statsEnabled = computed(() => state.hackerMode.value)
  const { stats: playbackStats } = usePlaybackStats(videoRef, statsEnabled)

  // Hacker mode also mirrors the scrub-preview engine log to the console
  // (prefix [ScrubPreview]) — frontend pump health vs provider fetch latency.
  watch(
    () => state.hackerMode.value,
    (v) => {
      scrubDebug.console = v
    },
    { immediate: true },
  )

  // Seek pipeline trace — what actually happens between releasing the scrubber
  // and playback resuming: buffer check (hit = instant, no network), pipeline
  // flush + segment fetch, decode from the nearest keyframe to the target
  // (the `seeked` event), then buffer refill to readyState ≥ 3 (resume).
  const lastSeek = ref<SeekTrace | null>(null)

  function traceSeekStart(target: number) {
    if (!state.hackerMode.value) return
    const v = videoRef.value
    if (!v) return
    let hit = false
    for (let i = 0; i < v.buffered.length; i++) {
      if (target >= v.buffered.start(i) && target <= v.buffered.end(i)) {
        hit = true
        break
      }
    }
    lastSeek.value = {
      target,
      bufferHit: hit,
      t0: performance.now(),
      fetchMs: null,
      fetchedRange: null,
      seekedMs: null,
      resumeMs: null,
      frags: 0,
      bytes: 0,
      done: false,
    }
  }

  // Fetch-phase depth: the `progress` event fires as bytes arrive — the moment
  // a buffered range covers the seek target, the network part of the seek is
  // done (mp4: the ranged bytes landed; hls: the segment(s) arrived).
  function onVideoProgress() {
    const s = lastSeek.value
    const v = videoRef.value
    if (!s || s.done || s.fetchMs !== null || !v) return
    for (let i = 0; i < v.buffered.length; i++) {
      const start = v.buffered.start(i)
      const end = v.buffered.end(i)
      if (s.target >= start && s.target <= end) {
        s.fetchMs = Math.round(performance.now() - s.t0)
        s.fetchedRange = [start, end]
        return
      }
    }
  }

  /** Seek trace: decoder positioned at the target frame (the `seeked` event). */
  function noteSeeked() {
    const s = lastSeek.value
    if (s && !s.done && s.seekedMs === null) {
      s.seekedMs = Math.round(performance.now() - s.t0)
    }
  }

  function markSeekResumed() {
    const s = lastSeek.value
    if (!s || s.done) return
    if (s.seekedMs === null) return // resume can't precede the seeked event
    s.resumeMs = Math.round(performance.now() - s.t0)
    s.done = true
  }

  // Count fragments fetched while a seek is in flight (hls path; FRAG_LOADED
  // appends exactly one entry per fragment to the rolling fragStats window).
  watch(engine.fragStats, (arr) => {
    const s = lastSeek.value
    if (!s || s.done) return
    const last = arr[arr.length - 1]
    if (!last) return
    s.frags++
    s.bytes += last.size
  })

  // HUD shows while paused or while actively buffering/seeking (or always when
  // pinned). When the show-condition drops (playback resumed), the panel
  // LINGERS ~1s, fades 0.4s, then unmounts — so the final seek numbers are
  // readable instead of vanishing the instant video continues.
  const hudCondition = computed(
    () =>
      state.hackerMode.value &&
      (state.hudPinned.value || !state.playing.value || showBuffering.value),
  )
  const hudVisible = ref(false)
  const hudFading = ref(false)
  let hudLingerTimer: ReturnType<typeof setTimeout> | null = null
  let hudFadeTimer: ReturnType<typeof setTimeout> | null = null

  function clearHudTimers() {
    if (hudLingerTimer) { clearTimeout(hudLingerTimer); hudLingerTimer = null }
    if (hudFadeTimer) { clearTimeout(hudFadeTimer); hudFadeTimer = null }
  }

  watch(hudCondition, (on) => {
    clearHudTimers()
    if (on) {
      hudVisible.value = true
      hudFading.value = false
      return
    }
    // linger 1s with full opacity, then 0.4s CSS fade, then unmount
    hudLingerTimer = setTimeout(() => {
      hudFading.value = true
      hudFadeTimer = setTimeout(() => {
        hudVisible.value = false
        hudFading.value = false
      }, 450)
    }, 1000)
  }, { immediate: true })

  // Scrub-bar heatmap segments — size-tinted (green <300KB, amber <1MB, red ≥1MB).
  const fragOverlay = computed(() => {
    if (!state.hackerMode.value) return []
    const dur = duration.value
    if (!dur) return []
    return engine.fragStats.value.map((f) => ({
      startPct: (f.start / dur) * 100,
      widthPct: (f.duration / dur) * 100,
      tone: (f.size < 300_000 ? 'ok' : f.size < 1_000_000 ? 'warn' : 'bad') as 'ok' | 'warn' | 'bad',
      label: `${Math.round(f.size / 1024)} KB · ${Math.round(f.loadMs)} ms`,
    }))
  })

  // Compact line set for the settings-menu mini-stats section.
  const debugStats = computed(() => {
    if (!state.hackerMode.value) return null
    const bwv = engine.bandwidthEstimate.value
    const frs = engine.fragStats.value
    const last = frs[frs.length - 1]
    const edge = engine.servedEdge.value
    const trail = engine.edgeTrail.value
    const ladderSnap = ladder.debugSnapshot()
    const conn = deps.getConnectionState?.() ?? 'ok'
    return {
      bw: bwv > 0 ? `${(bwv / 1_000_000).toFixed(1)} Mbit/s` : '—',
      conn: conn === 'ok' ? 'ok' : conn,

      buffer: `+${playbackStats.value.bufferAheadSec.toFixed(1)}s / −${playbackStats.value.bufferBehindSec.toFixed(1)}s`,
      level:
        engine.currentLevelLabel.value ||
        (currentStream.value?.type === 'mp4' ? 'mp4' : '—'),
      frag: last ? `${Math.round(last.size / 1024)} KB · ${Math.round(last.loadMs)} ms` : '—',
      // Kodik/solodcdn edge telemetry — empty for every other source (no header).
      // The decision (served edge) + the logic/metrics (attempt trail, rotations).
      edge,
      edgeTrail: edge ? formatEdgeTrail(trail) : '',
      edgeRot: edge && trail ? trail.split(',').length - 1 : 0,
      // Protocol-ladder telemetry (multi-tier prod only; null snapshot in dev).
      // I4: tier display is 1-based ("tier 2/3") via formatLadderRows — the
      // underlying debugSnapshot().tierIndex contract stays 0-based.
      ...(ladderSnap ? formatLadderRows(ladderSnap) : {}),
    }
  })

  return {
    playbackStats,
    lastSeek,
    traceSeekStart,
    onVideoProgress,
    noteSeeked,
    markSeekResumed,
    hudVisible,
    hudFading,
    clearHudTimers,
    fragOverlay,
    debugStats,
  }
}
