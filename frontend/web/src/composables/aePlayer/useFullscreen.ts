import { computed, type ComputedRef, type Ref } from 'vue'

// ── Fullscreen (capability-based) ────────────────────────────────────────────
// Android/desktop: native element fullscreen (+ landscape lock on touch).
// iPhone Safari has NO element fullscreen API — fall back to a fixed-position
// pseudo-fullscreen takeover. video.webkitEnterFullscreen() (the native iOS
// player) is deliberately NOT used: it drops SubtitleOverlay, the Source
// panel and the WT button.

export interface FullscreenDeps {
  rootRef: Ref<HTMLElement | null>
  videoRef: Ref<HTMLVideoElement | null>
  isCoarse: ComputedRef<boolean> | Ref<boolean>
  /** Component-owned so earlier clusters (mobile sheets) can read them. */
  pseudoFs: Ref<boolean>
  nativeFsActive: Ref<boolean>
}

export function useFullscreen(deps: FullscreenDeps) {
  const { rootRef, videoRef, isCoarse, pseudoFs, nativeFsActive } = deps

  const fullscreenActive = computed(() => nativeFsActive.value || pseudoFs.value)
  // Tracks whether WE pushed the pseudo-FS history entry and it hasn't been
  // consumed yet — `history.state` itself is unreliable because url-sync
  // (router.replace on episode/provider change) can overwrite the top entry's
  // state while pseudo-FS is active, dropping our marker.
  let pseudoFsEntryPushed = false

  function onFullscreenChange() {
    nativeFsActive.value = !!document.fullscreenElement
    if (!nativeFsActive.value) unlockOrientation()
  }

  function lockLandscape() {
    const o = screen.orientation as ScreenOrientation & { lock?: (v: string) => Promise<void> }
    void o?.lock?.('landscape').catch(() => {})
  }

  function unlockOrientation() {
    try {
      screen.orientation?.unlock?.()
    } catch {
      /* not locked / unsupported */
    }
  }

  function onToggleFullscreen() {
    const el = rootRef.value ?? videoRef.value?.parentElement
    if (!el) return
    if (document.fullscreenElement) {
      void document.exitFullscreen()
      return
    }
    if (pseudoFs.value) {
      exitPseudoFs()
      return
    }
    if (el.requestFullscreen) {
      el
        .requestFullscreen()
        .then(() => {
          if (isCoarse.value) lockLandscape()
        })
        .catch(() => {
          // Some WebKit builds (notably iPhone Safari) expose requestFullscreen
          // as a function but reject it at call time for non-<video> elements —
          // the feature-detect above lies. Fall back to the CSS takeover.
          enterPseudoFs()
        })
      return
    }
    enterPseudoFs()
  }

  // Pseudo-FS pushes a history entry so the phone's back gesture exits the
  // takeover instead of leaving the page.
  function onPseudoFsPop() {
    pseudoFsEntryPushed = false // the entry was just consumed by this pop
    exitPseudoFs(true)
  }

  function enterPseudoFs() {
    pseudoFs.value = true
    document.documentElement.classList.add('pl-noscroll')
    // Merge with the existing state so vue-router's own bookkeeping
    // ({position, back, current…}) survives alongside our marker.
    history.pushState({ ...history.state, plPseudoFs: true }, '')
    pseudoFsEntryPushed = true
    window.addEventListener('popstate', onPseudoFsPop)
  }

  function exitPseudoFs(viaPop = false) {
    if (!pseudoFs.value) return
    pseudoFs.value = false
    document.documentElement.classList.remove('pl-noscroll')
    window.removeEventListener('popstate', onPseudoFsPop)
    if (!viaPop && pseudoFsEntryPushed) {
      pseudoFsEntryPushed = false
      history.back()
    }
  }

  /** Unmount-safe teardown: never touches history (a route change already moved it). */
  function teardownPseudoFs() {
    if (!pseudoFs.value) return
    pseudoFs.value = false
    document.documentElement.classList.remove('pl-noscroll')
    window.removeEventListener('popstate', onPseudoFsPop)
    pseudoFsEntryPushed = false
  }

  return { fullscreenActive, onToggleFullscreen, onFullscreenChange, teardownPseudoFs }
}
