import { watch, type ComputedRef, type Ref } from 'vue'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { MenuKind } from '@/composables/aePlayer/usePlayerMenus'

// ── Controls auto-hide (idle while playing) ──────────────────────────────────
// Top bar + control bar fade out after UI_IDLE_MS of pointer inactivity while
// playing (matters most in fullscreen). Any pointer/keyboard activity, a pause,
// or an open menu brings them back and keeps them visible.

export interface UiIdleDeps {
  state: PlayerState
  uiVisible: Ref<boolean>
  isPointerInside: Ref<boolean>
  openMenu: Ref<MenuKind>
  isCoarse: ComputedRef<boolean> | Ref<boolean>
  videoRef: Ref<HTMLVideoElement | null>
}

const UI_IDLE_MS = 2500

export function useUiIdle(deps: UiIdleDeps) {
  const { state, uiVisible, isPointerInside, openMenu, isCoarse, videoRef } = deps

  let uiIdleTimer: ReturnType<typeof setTimeout> | null = null

  function clearUiIdleTimer() {
    if (uiIdleTimer !== null) {
      clearTimeout(uiIdleTimer)
      uiIdleTimer = null
    }
  }

  function armUiIdleTimer() {
    clearUiIdleTimer()
    if (!state.playing.value || openMenu.value !== null) return
    uiIdleTimer = setTimeout(() => {
      uiVisible.value = false
    }, UI_IDLE_MS)
  }

  function wakeUi() {
    uiVisible.value = true
    armUiIdleTimer()
  }

  // On touch the video tap handler owns chrome toggling — pre-waking from the
  // root touchstart would make every tap see "chrome already visible" and
  // immediately hide it again (toggle would never show the chrome).
  function onRootTouch(e: TouchEvent) {
    if (isCoarse.value && e.target === videoRef.value) return
    wakeUi()
  }

  function onPointerEnter() {
    isPointerInside.value = true
    wakeUi()
  }

  function onPointerLeave() {
    isPointerInside.value = false
    // Pointer left the player while playing — hide right away (menus pin it)
    if (state.playing.value && openMenu.value === null) {
      clearUiIdleTimer()
      uiVisible.value = false
    }
  }

  watch(openMenu, (menu) => {
    if (menu !== null) {
      clearUiIdleTimer()
      uiVisible.value = true
    } else {
      armUiIdleTimer()
    }
  })

  return {
    clearUiIdleTimer,
    armUiIdleTimer,
    wakeUi,
    onRootTouch,
    onPointerEnter,
    onPointerLeave,
  }
}
