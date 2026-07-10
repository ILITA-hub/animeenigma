/**
 * Workstream watch-together — Phase 3 (player-sync) Plan 03.1.
 *
 * `usePlayerSyncBridge(videoRef, room)` — generic bridge between an HTML5
 * `<video>` element and a `WatchTogetherRoomHandle`. Wires bidirectional
 * playback sync for any of the four HTML5-based players (AnimeLib,
 * OurEnglish, Hanime, Raw). The Kodik adapter (Wave 2 plan 03.4) uses a
 * separate iframe postMessage RPC path and does NOT consume this composable.
 *
 *   ┌──────────────────┐  emitPlay/Pause/Seek    ┌─────────────────────┐
 *   │  <video>         │ ───────────────────────▶│  room handle        │
 *   │  native events ──┤                         │  (server via WS)    │
 *   │  play/pause/seeked│  apply remote events   │                     │
 *   │  & timeTick loop ◀──────────────────────── │                     │
 *   └──────────────────┘                         └─────────────────────┘
 *
 * The bridge guards against echo storms: when a remote `playback:event`
 * triggers a programmatic `video.play()`, the browser fires a native `play`
 * event which the bridge MUST NOT re-emit. The `applyingRemote` flag — set
 * around every programmatic side-effect, cleared by either the natural
 * follow-up event or a 250ms watchdog timer — handles this. A secondary
 * `lastAppliedTime` heuristic catches stray `seeked` events.
 *
 * Drift correction is per WT-SYNC-06:
 *   - Soft (|drift| < 1.0s) → playbackRate nudge 0.97/1.03 for 5s, then restore
 *   - Hard (|drift| >= 1.0s) → direct `currentTime = target`
 *
 * Time-tick heartbeat (WT-SYNC-05) runs at 1Hz via a single
 * `requestAnimationFrame` loop gated by `Date.now() - lastTickAt >= 1000`
 * — never a wall-clock timer interval (RAF auto-throttles in background
 * tabs and avoids timer drift).
 *
 * Call site (consumer): a single line inside a player component's setup:
 *   if (props.room) usePlayerSyncBridge(videoRef, props.room)
 *
 * Zero behavior change when `props.room` is null/undefined — the consumer
 * gates the call, so the bridge itself NEVER defends against a null room.
 */

import { onBeforeUnmount, watch, type Ref } from 'vue'

import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import type {
  PlaybackCorrectionData,
  PlaybackEventData,
} from '@/types/watch-together'

/* ──────────────────────────────────────────────────────────────────────── */
/*  Constants — single source of truth for thresholds + intervals.          */
/* ──────────────────────────────────────────────────────────────────────── */

/** Drift threshold (seconds) — below = soft nudge, at-or-above = hard seek. */
const DRIFT_SOFT_THRESHOLD = 1.0

/** Time-tick heartbeat cadence (ms). 1Hz per WT-SYNC-05. */
const TICK_INTERVAL_MS = 1000

/** Apply-guard fallback clear timeout (ms). Native echo event usually beats this. */
const APPLY_GUARD_TIMEOUT_MS = 250

/** playbackRate values for soft correction. */
const SOFT_CORRECTION_RATE_SLOW = 0.97
const SOFT_CORRECTION_RATE_FAST = 1.03

/** Soft-correction nudge duration (ms). After this, playbackRate returns to 1.0. */
const SOFT_CORRECTION_DURATION_MS = 5000

/** Tolerance (seconds) for the secondary echo guard on `seeked`. */
const SEEK_GUARD_TOLERANCE = 0.5

/* ──────────────────────────────────────────────────────────────────────── */
/*  Helpers.                                                                */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Compute the target playback time for a correction, compensating for the
 * server→client network latency by adding `(Date.now() - server_ts) / 1000`.
 * Server timestamps are unix ms; result is seconds into the episode.
 */
function computeTargetTime(correctionTime: number, serverTs: number): number {
  return correctionTime + (Date.now() - serverTs) / 1000
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Composable.                                                             */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Wire `videoRef` ↔ `room` bidirectional playback sync. Returns nothing —
 * the bridge is purely side-effectful. Call once per player component setup.
 *
 * Lifecycle: native listeners attach when `videoRef.value` first transitions
 * non-null, detach when it transitions back to null or the parent component
 * unmounts. Room subscriptions register immediately and unsubscribe on
 * `onBeforeUnmount`.
 */
export function usePlayerSyncBridge(
  videoRef: Ref<HTMLVideoElement | null>,
  room: WatchTogetherRoomHandle,
  opts?: {
    /** Called when a remote-driven play() is rejected (e.g. NotAllowedError
     *  from the browser's autoplay policy) so the host player can surface its
     *  blocked-playback UI instead of silently desyncing from the room. */
    onPlayRejected?: (err: unknown) => void
  },
): void {
  // ── Closure-scoped state (one bridge instance per player) ──

  /** True while a programmatic video call is in-flight; suppresses local re-emit. */
  let applyingRemote = false

  /** Fallback clear timer when the native echo event never fires. */
  let applyingRemoteClearTimer: ReturnType<typeof setTimeout> | null = null

  /** Last currentTime we applied from a remote event (for secondary seek-echo guard). */
  let lastAppliedTime = -1

  /** Wall-clock ms of the last `emitTimeTick`. */
  let lastTickAt = 0

  /** Active RAF handle (null while the tick loop is stopped). */
  let rafId: number | null = null

  /** Pending soft-correction restore timer (clears + restores playbackRate). */
  let playbackRateRestoreTimer: ReturnType<typeof setTimeout> | null = null

  /** Unsubscribe closures collected on room.on*; drained on unmount. */
  const unsubscribers: Array<() => void> = []

  /** Track the currently-bound HTMLVideoElement so we can detach cleanly. */
  let boundElement: HTMLVideoElement | null = null

  /* ────── Apply-guard helper ────── */

  function clearApplyingRemote(): void {
    applyingRemote = false
    if (applyingRemoteClearTimer !== null) {
      clearTimeout(applyingRemoteClearTimer)
      applyingRemoteClearTimer = null
    }
  }

  function withApplyGuard(fn: () => void): void {
    applyingRemote = true
    if (applyingRemoteClearTimer !== null) {
      clearTimeout(applyingRemoteClearTimer)
    }
    try {
      fn()
    } finally {
      applyingRemoteClearTimer = setTimeout(() => {
        applyingRemoteClearTimer = null
        applyingRemote = false
      }, APPLY_GUARD_TIMEOUT_MS)
    }
  }

  /* ────── Tick loop (RAF-driven, gated by Date.now() delta) ────── */

  function tickLoop(): void {
    // Re-schedule first — even if we early-return below, we want the loop alive
    // so the next iteration can re-check `paused`/`ended`/`seeking`.
    rafId = requestAnimationFrame(tickLoop)
    const v = videoRef.value
    if (!v || v.paused || v.ended || v.seeking) return
    const now = Date.now()
    if (now - lastTickAt < TICK_INTERVAL_MS) return
    lastTickAt = now
    room.emitTimeTick(v.currentTime)
  }

  function startTickLoop(): void {
    if (rafId !== null) return
    // Reset lastTickAt so the first tick fires after a fresh 1s window from
    // "start playing", not immediately (which would double-tick on a play
    // event that already fired close in time to a previous tick).
    lastTickAt = Date.now()
    rafId = requestAnimationFrame(tickLoop)
  }

  function stopTickLoop(): void {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
  }

  /* ────── Native event handlers ────── */

  function handlePlay(): void {
    if (applyingRemote) return
    const v = videoRef.value
    if (!v) return
    room.emitPlay(v.currentTime)
    startTickLoop()
  }

  function handlePause(): void {
    if (applyingRemote) return
    const v = videoRef.value
    if (!v) return
    room.emitPause(v.currentTime)
    stopTickLoop()
  }

  function handleSeeked(): void {
    if (applyingRemote) return
    const v = videoRef.value
    if (!v) return
    // Secondary echo guard: a `seeked` event firing within ±0.5s of the
    // most-recently-applied remote time is almost certainly the browser's
    // natural follow-up to a programmatic `currentTime` write, not a real
    // user seek. Swallow.
    if (
      lastAppliedTime >= 0 &&
      Math.abs(v.currentTime - lastAppliedTime) < SEEK_GUARD_TOLERANCE
    ) {
      return
    }
    room.emitSeek(v.currentTime)
  }

  function handleEnded(): void {
    // No emit — server can derive "ended" from time vs duration if it ever
    // needs to; v1 doesn't. Just stop ticking.
    stopTickLoop()
  }

  /* ────── Element attach / detach ────── */

  function attachListeners(el: HTMLVideoElement): void {
    el.addEventListener('play', handlePlay)
    el.addEventListener('pause', handlePause)
    el.addEventListener('seeked', handleSeeked)
    el.addEventListener('ended', handleEnded)
    boundElement = el
  }

  function detachListeners(): void {
    if (!boundElement) return
    boundElement.removeEventListener('play', handlePlay)
    boundElement.removeEventListener('pause', handlePause)
    boundElement.removeEventListener('seeked', handleSeeked)
    boundElement.removeEventListener('ended', handleEnded)
    boundElement = null
  }

  /* ────── Remote subscription handlers ────── */

  const unsubPlayback = room.onPlaybackEvent((e: PlaybackEventData) => {
    const v = videoRef.value
    if (!v) return
    switch (e.kind) {
      case 'play': {
        withApplyGuard(() => {
          if (Math.abs(v.currentTime - e.time) > DRIFT_SOFT_THRESHOLD) {
            v.currentTime = e.time
            lastAppliedTime = e.time
          }
          // Forward a play() rejection (autoplay policy etc.) to the host
          // player so it can raise its blocked-playback overlay — a silently
          // vetoed play() would leave this member desynced from the room.
          v.play().catch((err: unknown) => opts?.onPlayRejected?.(err))
        })
        startTickLoop()
        return
      }
      case 'pause': {
        withApplyGuard(() => {
          v.pause()
          v.currentTime = e.time
          lastAppliedTime = e.time
        })
        stopTickLoop()
        return
      }
      case 'seek': {
        withApplyGuard(() => {
          v.currentTime = e.time
          lastAppliedTime = e.time
        })
        return
      }
    }
  })
  unsubscribers.push(unsubPlayback)

  const unsubCorrection = room.onCorrection((c: PlaybackCorrectionData) => {
    const v = videoRef.value
    if (!v) return
    const target = computeTargetTime(c.time, c.server_ts)
    const drift = Math.abs(v.currentTime - target)
    if (drift < DRIFT_SOFT_THRESHOLD) {
      // Soft nudge — adjust playbackRate, restore after 5s.
      v.playbackRate =
        v.currentTime < target ? SOFT_CORRECTION_RATE_FAST : SOFT_CORRECTION_RATE_SLOW
      if (playbackRateRestoreTimer !== null) {
        clearTimeout(playbackRateRestoreTimer)
      }
      playbackRateRestoreTimer = setTimeout(() => {
        playbackRateRestoreTimer = null
        const cur = videoRef.value
        if (cur) cur.playbackRate = 1.0
      }, SOFT_CORRECTION_DURATION_MS)
      return
    }
    // Hard correction — direct seek. Silent per WT-SYNC-06 (no toast).
    withApplyGuard(() => {
      v.currentTime = target
      lastAppliedTime = target
    })
  })
  unsubscribers.push(unsubCorrection)

  /* ────── Watch videoRef for null↔element transitions ────── */

  watch(
    videoRef,
    (next, prev) => {
      // Element going away → detach.
      if (prev && prev !== next) {
        detachListeners()
        stopTickLoop()
      }
      // New element → attach.
      if (next && next !== boundElement) {
        attachListeners(next)
      }
    },
    { immediate: true },
  )

  /* ────── Vue lifecycle: full cleanup ────── */

  onBeforeUnmount(() => {
    stopTickLoop()
    if (applyingRemoteClearTimer !== null) {
      clearTimeout(applyingRemoteClearTimer)
      applyingRemoteClearTimer = null
    }
    if (playbackRateRestoreTimer !== null) {
      clearTimeout(playbackRateRestoreTimer)
      playbackRateRestoreTimer = null
    }
    // Restore playbackRate so the next mount of the same video element
    // doesn't start in a mid-correction state.
    const v = videoRef.value
    if (v && v.playbackRate !== 1.0) {
      v.playbackRate = 1.0
    }
    detachListeners()
    for (const unsub of unsubscribers) {
      try {
        unsub()
      } catch {
        // best-effort — never let cleanup throw across the unmount boundary
      }
    }
    unsubscribers.length = 0
    // Mark idle so any late callback that somehow re-enters is a no-op.
    clearApplyingRemote()
  })
}
