import { ref, type Ref } from 'vue'
import { recordPlayerEvent } from '@/utils/playerTelemetry'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'

// ── Autoplay-blocked state ────────────────────────────────────────────────────
// Any browser can reject video.play() with NotAllowedError — strict autoplay
// policies (Firefox "Block Audio and Video", Chrome's engagement heuristics,
// Safari power-saving), blocker extensions, or a play() that lands outside the
// user-gesture window (our resolves are async, so the click's activation can
// expire before play() runs). The media itself is FINE — segments/bytes load —
// only STARTING is vetoed. These rejections used to be swallowed
// (`void v.play()`), so affected users saw a dead player while the stall
// watchdog churned through every source. All play() calls now funnel through
// attemptPlay(); a NotAllowedError raises this dedicated overlay and must
// NEVER count as a dead source.

export interface AutoplayGateDeps {
  videoRef: Ref<HTMLVideoElement | null>
  state: PlayerState
  getAnimeId: () => string
  getEpisodeNumber: () => number | undefined
}

export function useAutoplayGate(deps: AutoplayGateDeps) {
  const { videoRef, state } = deps

  const playbackBlocked = ref(false)
  // Second consecutive rejection (the overlay's own button also got vetoed) →
  // show the browser-permission hint.
  const playbackBlockedHint = ref(false)
  let blockReported = false // one playback_start_rejected event per resolve

  function handlePlayRejection(err: unknown) {
    const name = err instanceof Error ? err.name : ''
    // AbortError (play() interrupted by a load/pause during source swaps) and
    // friends are benign lifecycle noise — only a browser start-veto matters.
    if (name !== 'NotAllowedError') return
    if (playbackBlocked.value) playbackBlockedHint.value = true
    playbackBlocked.value = true
    if (!blockReported) {
      blockReported = true
      recordPlayerEvent({
        kind: 'playback_start_rejected',
        provider: state.combo.value.provider,
        anime_id: deps.getAnimeId(),
        episode: deps.getEpisodeNumber(),
        error_kind: name,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
    }
    const msg = err instanceof Error && err.message ? `: ${err.message}` : ''
    console.warn(`[AePlayer] play() rejected — ${name}${msg}`)
  }

  function resetPlaybackBlocked() {
    playbackBlocked.value = false
    playbackBlockedHint.value = false
    blockReported = false
  }

  // The single sanctioned way to start playback. Success clears the blocked
  // overlay (the user allowed autoplay / the veto lifted); a veto raises it.
  function attemptPlay() {
    const v = videoRef.value
    if (!v) return
    v.play().then(
      () => {
        playbackBlocked.value = false
        playbackBlockedHint.value = false
      },
      handlePlayRejection,
    )
  }

  return {
    playbackBlocked,
    playbackBlockedHint,
    handlePlayRejection,
    resetPlaybackBlocked,
    attemptPlay,
  }
}
