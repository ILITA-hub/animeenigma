import { ref, type Ref } from 'vue'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'

// ── rAF progress loop + playhead preservation ────────────────────────────────
// Owns the reactive playback clock (currentTime/duration/bufferedPct/
// hasStarted), the rAF tick that syncs it from the <video> element, and the
// capture/restore pair that keeps the viewer's position across same-episode
// stream swaps.

export interface PlaybackClockDeps {
  videoRef: Ref<HTMLVideoElement | null>
  state: PlayerState
  engine: ReturnType<typeof useVideoEngine>
  /** True once the playhead reaches the episode end (owned by the component,
   *  shared with the next-episode cluster) — reset at every source swap. */
  reachedEpisodeEnd: Ref<boolean>
  attemptPlay: () => void
  /** Watch-tracking heartbeat — late-bound (tracking is created after this). */
  trackingTick: (timeSec: number, durationSec: number) => void
  trackingSaveNow: () => void
  /** Controls auto-hide wiring — late-bound (UI-idle cluster comes later). */
  armUiIdleTimer: () => void
  clearUiIdleTimer: () => void
  uiVisible: Ref<boolean>
}

/** rAF-path snap rates (see writeProgress) — mirrors SubtitleOverlay's
 *  TIME_SYNC_HZ pattern. 4 Hz time / half-percent buffered ≈ sub-pixel on
 *  the scrub bar while cutting reactive re-renders ~15×. */
const PROGRESS_SYNC_HZ = 4
const BUFFERED_SYNC_STEPS_PER_PCT = 2

export function usePlaybackClock(deps: PlaybackClockDeps) {
  const { videoRef, state, engine, reachedEpisodeEnd } = deps

  const currentTime = ref(0)
  const duration = ref(0)
  const bufferedPct = ref(0)
  /** true once playback has started for the current stream — gates the poster */
  const hasStarted = ref(false)
  let rafId: number | null = null

  // Clear all playback-derived reactive state at a source swap. currentTime.value
  // is synced from the <video> element ONLY by the rAF loop (while playing) and
  // the final writeProgress() on pause — neither runs between attaching a new
  // source and the first play. Without this, the OUTGOING source's playhead
  // lingers in currentTime.value while the incoming source sits paused at 0, so
  // currentTime-derived UI reads a stale position: e.g. a viewer parked in the
  // ending, then switching server/episode, saw the "Skip Ending" chip render
  // before pressing play (activeSkipSegment saw the stale ~ending-window time).
  // restorePlayhead() re-seats currentTime.value when a same-episode swap keeps
  // the position; a fresh (episode-change) load leaves it at 0.
  function resetPlaybackClock() {
    currentTime.value = 0
    duration.value = 0
    bufferedPct.value = 0
    state.progress.value = 0
    // The incoming source hasn't reached its own end — drop any lingering
    // end-of-episode flag so the next-episode chip doesn't leak across the swap
    // (same stale-state class as the Skip-Ending chip above).
    reachedEpisodeEnd.value = false
  }

  function writeProgress(quantize = false) {
    const v = videoRef.value
    if (!v) return
    // Change-gate every reactive write, and QUANTIZE the fast movers on the rAF
    // path: currentTime moves every frame, so the change-gate never held during
    // playback and the control bar re-rendered 60×/sec (style recalc + layout
    // per frame — the 2026-07-04 render trace). Snapping time to 250ms and
    // buffered to 0.5% drops reactive updates to ~4 Hz, visually
    // indistinguishable on the scrub bar. Event-driven callers (seek, pause)
    // stay exact so a paused UI is frame-accurate. SubtitleOverlay syncs off
    // the <video> element directly (own rAF + snap grid) and is unaffected.
    const t = quantize ? Math.floor(v.currentTime * PROGRESS_SYNC_HZ) / PROGRESS_SYNC_HZ : v.currentTime
    if (t !== currentTime.value) currentTime.value = t
    const dur = v.duration || 0
    if (dur !== duration.value) duration.value = dur
    if (dur > 0) {
      const pct = (t / dur) * 100
      if (pct !== state.progress.value) state.progress.value = pct
    }
    // Buffered
    if (v.buffered.length > 0 && dur > 0) {
      const bpct = (v.buffered.end(v.buffered.length - 1) / dur) * 100
      const qb = quantize ? Math.floor(bpct * BUFFERED_SYNC_STEPS_PER_PCT) / BUFFERED_SYNC_STEPS_PER_PCT : bpct
      if (qb !== bufferedPct.value) bufferedPct.value = qb
    }
    // Watch tracking: heartbeat saves + duration-aware auto-complete — always
    // fed the RAW position (it does its own gating). Only feed real playback
    // positions — a paused pre-start frame (currentTime 0) or a dead source
    // must not write progress.
    if (v.currentTime > 0) {
      deps.trackingTick(v.currentTime, dur)
    }
  }

  function tick() {
    writeProgress(true)
    rafId = requestAnimationFrame(tick)
  }

  function startRaf() {
    if (rafId === null) {
      rafId = requestAnimationFrame(tick)
    }
  }

  function stopRaf() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
    // One final write so progress/time are up-to-date while paused
    writeProgress()
  }

  function onVideoPlay() {
    state.playing.value = true
    reachedEpisodeEnd.value = false // playback resumed — the end chip is stale
    startRaf()
    deps.armUiIdleTimer()
  }

  function onVideoPause() {
    state.playing.value = false
    stopRaf() // final writeProgress() inside keeps tracking's lastKnown fresh
    deps.trackingSaveNow()
    deps.clearUiIdleTimer()
    deps.uiVisible.value = true
  }

  // Restore the playhead (and play state) after a stream swap that stayed on the
  // same episode — a provider/facet/quality switch must not dump the viewer back
  // to 0:00. Restoring the real position ALSO stops the regressed near-zero
  // playhead from clobbering saved watch-progress on the next heartbeat/beacon
  // (both persist the CURRENT playhead, not the max), which is why "resume from
  // the same moment" vanished after a mid-watch source switch. No-op when there's
  // nothing to restore (a fresh load sits at 0).
  function restorePlayhead(t: number, wasPlaying: boolean) {
    const v = videoRef.value
    if (!v || t <= 0.5) return
    const restore = () => {
      try {
        v.currentTime = t
        // Keep the reactive clock in step with the seeked element: resetPlaybackClock
        // zeroed it for the swap, and a PAUSED preserve fires no rAF/writeProgress to
        // re-sync, so the scrub bar / time pill would otherwise read 0:00 at position t.
        currentTime.value = t
      } catch {
        /* not seekable yet — best-effort */
      }
      if (wasPlaying) deps.attemptPlay()
    }
    if (v.readyState >= 1) restore()
    else v.addEventListener('loadedmetadata', restore, { once: true })
  }

  // Snapshot the playhead + play state to hand to restorePlayhead after a
  // same-episode stream swap. `keep` is false for a genuine episode change (start
  // fresh at 0); wasPlaying is gated on a real position so a fresh 0:00 load never
  // auto-resumes. Prefer engine.lastKnownPlayback when a fatal error just fired:
  // hls.js's destroy() already reset videoRef's own currentTime to 0 by the time
  // a retry/failover reaches here (see useVideoEngine's snapshotPlayback), so the
  // live element is not a safe read in that case.
  function capturePlayhead(keep: boolean): { restoreT: number; wasPlaying: boolean } {
    if (!keep) return { restoreT: 0, wasPlaying: false }
    const salvaged = engine.lastKnownPlayback.value
    const restoreT = salvaged?.time ?? videoRef.value?.currentTime ?? 0
    const wasPlaying = restoreT > 0 && (salvaged?.wasPlaying ?? state.playing.value)
    return { restoreT, wasPlaying }
  }

  return {
    currentTime,
    duration,
    bufferedPct,
    hasStarted,
    resetPlaybackClock,
    writeProgress,
    stopRaf,
    onVideoPlay,
    onVideoPause,
    restorePlayhead,
    capturePlayhead,
  }
}
