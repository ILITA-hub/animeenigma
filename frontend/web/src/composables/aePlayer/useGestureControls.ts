import { ref, type ComputedRef, type Ref } from 'vue'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { MenuKind } from '@/composables/aePlayer/usePlayerMenus'

// ── Playback helpers: tap gestures, volume/speed/PiP, seeking ────────────────
// A tap on the video: backdrop-dismiss if a menu is open (no side effect).
// Desktop click = play/pause. Touch (coarse): single tap toggles the chrome,
// double-tap on the side thirds seeks ±10s (center double-tap = play/pause) —
// so the play/pause affordance on phones is the center overlay button, never
// a stray tap.

export interface GestureControlsDeps {
  videoRef: Ref<HTMLVideoElement | null>
  rootRef: Ref<HTMLElement | null>
  state: PlayerState
  isCoarse: ComputedRef<boolean> | Ref<boolean>
  openMenu: Ref<MenuKind>
  browseOpen: Ref<boolean>
  closeMenus: () => void
  uiVisible: Ref<boolean>
  wakeUi: () => void
  clearUiIdleTimer: () => void
  attemptPlay: () => void
  traceSeekStart: (target: number) => void
  writeProgress: () => void
}

const DOUBLE_TAP_MS = 280

export function useGestureControls(deps: GestureControlsDeps) {
  const { videoRef, rootRef, state, isCoarse, openMenu, browseOpen, uiVisible } = deps

  let lastTapAt = 0
  let lastTapX = 0
  let singleTapTimer: ReturnType<typeof setTimeout> | null = null

  const seekFlash = ref<'back' | 'fwd' | null>(null)
  let seekFlashTimer: ReturnType<typeof setTimeout> | null = null

  function flashSeek(dir: 'back' | 'fwd') {
    seekFlash.value = dir
    if (seekFlashTimer) clearTimeout(seekFlashTimer)
    seekFlashTimer = setTimeout(() => {
      seekFlash.value = null
    }, 500)
  }

  function onVideoClick(e: MouseEvent) {
    if (openMenu.value !== null || browseOpen.value) {
      deps.closeMenus()
      return
    }
    if (!isCoarse.value) {
      togglePlay()
      return
    }

    const now = performance.now()
    const isDouble = now - lastTapAt < DOUBLE_TAP_MS && Math.abs(e.clientX - lastTapX) < 64
    lastTapAt = now
    lastTapX = e.clientX

    if (isDouble) {
      if (singleTapTimer) {
        clearTimeout(singleTapTimer)
        singleTapTimer = null
      }
      lastTapAt = 0
      const rect = rootRef.value?.getBoundingClientRect()
      const x = rect && rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5
      if (x < 0.4) {
        onSeekRel(-10)
        flashSeek('back')
      } else if (x > 0.6) {
        onSeekRel(10)
        flashSeek('fwd')
      } else {
        togglePlay()
      }
      return
    }

    singleTapTimer = setTimeout(() => {
      singleTapTimer = null
      if (uiVisible.value && state.playing.value) {
        deps.clearUiIdleTimer()
        uiVisible.value = false
      } else {
        deps.wakeUi()
      }
    }, DOUBLE_TAP_MS)
  }

  function togglePlay() {
    const v = videoRef.value
    if (!v) return
    if (v.paused) {
      deps.attemptPlay()
    } else {
      v.pause()
    }
  }

  function onSeekRel(delta: number) {
    const v = videoRef.value
    if (!v) return
    const target = Math.max(0, Math.min(isFinite(v.duration) ? v.duration : Infinity, v.currentTime + delta))
    deps.traceSeekStart(target)
    v.currentTime = target
  }

  function onSeek(pct: number) {
    const v = videoRef.value
    if (!v || !v.duration) return
    const target = (pct / 100) * v.duration
    deps.traceSeekStart(target)
    v.currentTime = target
    // Write progress immediately so the scrub bar reflects the new position
    // even while paused (rAF loop is stopped when paused).
    deps.writeProgress()
  }

  function onSetVolume(vol: number) {
    state.volume.value = vol
    const v = videoRef.value
    if (v) v.volume = vol / 100
  }

  function onToggleMute() {
    state.muted.value = !state.muted.value
    const v = videoRef.value
    if (v) v.muted = state.muted.value
  }

  function onSetSpeed(speed: number) {
    state.speed.value = speed
    const v = videoRef.value
    if (v) v.playbackRate = speed
  }

  function onVolumeChange() {
    const v = videoRef.value
    if (!v) return
    // Sync state from element — covers PiP / media-session external changes.
    // Only write state here; the set-volume path writes to the element.
    state.volume.value = Math.round(v.volume * 100)
    state.muted.value = v.muted
  }

  function onTogglePip() {
    const v = videoRef.value
    if (!v) return
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture()
    } else {
      void v.requestPictureInPicture?.()
    }
  }

  function clearGestureTimers() {
    if (singleTapTimer) clearTimeout(singleTapTimer)
    if (seekFlashTimer) clearTimeout(seekFlashTimer)
  }

  return {
    seekFlash,
    onVideoClick,
    togglePlay,
    onSeekRel,
    onSeek,
    onSetVolume,
    onToggleMute,
    onSetSpeed,
    onVolumeChange,
    onTogglePip,
    clearGestureTimers,
  }
}
