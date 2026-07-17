import { computed, type ComputedRef, type Ref } from 'vue'

// ── Fullscreen (capability-based) ────────────────────────────────────────────
// Android/desktop/iPad: native element fullscreen (+ landscape lock on touch).
// iPhone Safari never yields a usable element fullscreen — the API is absent on
// older builds and present-but-lying on newer ones (it resolves for <video> and
// fails for everything else). Probing it and reacting to the failure is not
// reliable: a build that returns undefined instead of a promise makes .then()
// throw synchronously, which kills the toggle outright. So iPhone treats the CSS
// takeover as its FIRST-CLASS fullscreen path, not a rescue after a failed bet.
// video.webkitEnterFullscreen() (the native iOS player) is deliberately NOT
// used: it drops SubtitleOverlay, the Source panel and the WT button.

/** iPhone/iPod: element fullscreen is unusable, so the CSS takeover IS the path. */
function prefersPseudoFullscreen(): boolean {
  return typeof navigator !== 'undefined' && /iP(hone|od)/.test(navigator.userAgent)
}

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
    if (prefersPseudoFullscreen() || !el.requestFullscreen) {
      enterPseudoFs()
      return
    }
    try {
      const req = el.requestFullscreen()
      // The spec returns a Promise, but WebKit builds that return undefined would
      // make .then() throw and leave the toggle dead — only chain when thenable.
      if (req && typeof req.then === 'function') {
        req
          .then(() => {
            if (isCoarse.value) lockLandscape()
          })
          .catch(() => enterPseudoFs())
      } else if (isCoarse.value) {
        lockLandscape()
      }
    } catch {
      enterPseudoFs()
    }
  }

  // Pseudo-FS pushes a history entry so the phone's back gesture exits the
  // takeover instead of leaving the page.
  function onPseudoFsPop() {
    pseudoFsEntryPushed = false // the entry was just consumed by this pop
    exitPseudoFs(true)
  }

  // Under-island coverage: viewport-fit=cover ships STATICALLY in index.html.
  // Runtime opt-in from here was tried twice (setAttribute, then whole-node
  // replacement) and iOS 26 ignored both on-device — do not resurrect that
  // mechanism; only a load-time static cover expands the layout viewport.
  // The takeover's overlay rows pad themselves back inside the safe area
  // (AePlayer.vue — incl. a :deep(.pl-controls) rule for the control bar).

  function enterPseudoFs() {
    pseudoFs.value = true
    document.documentElement.classList.add('pl-noscroll')
    // Merge with the existing state so vue-router's own bookkeeping
    // ({position, back, current…}) survives alongside our marker.
    history.pushState({ ...history.state, plPseudoFs: true }, '')
    pseudoFsEntryPushed = true
    window.addEventListener('popstate', onPseudoFsPop)
  }

  /** Shared takeover teardown; callers handle history themselves. */
  function releasePseudoFs(): boolean {
    if (!pseudoFs.value) return false
    pseudoFs.value = false
    document.documentElement.classList.remove('pl-noscroll')
    window.removeEventListener('popstate', onPseudoFsPop)
    return true
  }

  function exitPseudoFs(viaPop = false) {
    if (!releasePseudoFs()) return
    if (!viaPop && pseudoFsEntryPushed) {
      pseudoFsEntryPushed = false
      history.back()
    }
  }

  /** Unmount-safe teardown: never touches history (a route change already moved it). */
  function teardownPseudoFs() {
    if (!releasePseudoFs()) return
    pseudoFsEntryPushed = false
  }

  return { fullscreenActive, onToggleFullscreen, onFullscreenChange, teardownPseudoFs }
}
