import { type ComputedRef, type Ref } from 'vue'
import { mapKeyToAction } from '@/composables/aePlayer/playerHotkeys'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { MenuKind } from '@/composables/aePlayer/usePlayerMenus'

// ── Keyboard shortcuts ────────────────────────────────────────────────────────
// Listen on window but only act after this player has become the keyboard owner.
// Pointer/focus/fullscreen activity claims ownership; an explicit interaction
// elsewhere releases it. Keeping that ownership across a transient body focus is
// important on Safari, which can reset activeElement while media keeps playing.
// (The window/document listeners are registered by the component lifecycle.)

export interface PlayerKeyboardDeps {
  rootRef: Ref<HTMLElement | null>
  videoRef: Ref<HTMLVideoElement | null>
  isPointerInside: Ref<boolean>
  state: PlayerState
  openMenu: Ref<MenuKind>
  browseOpen: Ref<boolean>
  wakeUi: () => void
  closeMenus: () => void
  toggleMenu: (menu: MenuKind) => void
  togglePlay: () => void
  onSeekRel: (delta: number) => void
  onSetVolume: (vol: number) => void
  onToggleMute: () => void
  onToggleFullscreen: () => void
  onTogglePip: () => void
  writeProgress: () => void
  anime_hasNextEp: ComputedRef<boolean>
  showNextEpisode: Ref<boolean>
  showNextEpChip: ComputedRef<boolean>
  goToNextEpisode: () => void
}

export function usePlayerKeyboard(deps: PlayerKeyboardDeps) {
  const { rootRef, videoRef, isPointerInside, state, openMenu, browseOpen } = deps
  let ownsKeyboard = false

  function fullscreenContains(root: HTMLElement): boolean {
    const fullscreenElement =
      document.fullscreenElement ||
      (document as Document & { webkitFullscreenElement?: Element | null }).webkitFullscreenElement
    return !!(fullscreenElement && (fullscreenElement === root || root.contains(fullscreenElement)))
  }

  function playerIsActive(): boolean {
    const root = rootRef.value
    if (!root) return false

    const activeElement = document.activeElement
    const activeNow =
      isPointerInside.value ||
      !!(activeElement && root.contains(activeElement)) ||
      fullscreenContains(root)

    if (activeNow) ownsKeyboard = true
    return activeNow || ownsKeyboard
  }

  function onDocumentPointerDown(e: PointerEvent) {
    const root = rootRef.value
    const target = e.target
    if (root && target instanceof Node && !root.contains(target)) ownsKeyboard = false
  }

  function onDocumentFocusIn(e: FocusEvent) {
    const root = rootRef.value
    const target = e.target
    if (!root || !(target instanceof Node) || root.contains(target)) return

    // WebKit may fall back to body/html when its media UI changes. That is not
    // a real user choice of another control, so it must not revoke ownership.
    if (target === document.body || target === document.documentElement) return
    ownsKeyboard = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (!playerIsActive()) return
    deps.wakeUi()

    if (e.key === 'Escape') {
      if (openMenu.value !== null || browseOpen.value) {
        deps.closeMenus()
        e.preventDefault()
      }
      return
    }

    const action = mapKeyToAction(e)
    if (!action) return
    e.preventDefault()

    switch (action.type) {
      case 'play-pause':
        deps.togglePlay()
        break
      case 'seek-rel':
        deps.onSeekRel(action.value)
        break
      case 'vol-rel': {
        const next = Math.max(0, Math.min(100, state.volume.value + action.value))
        if (state.muted.value && action.value > 0) deps.onToggleMute()
        deps.onSetVolume(next)
        break
      }
      case 'seek-pct': {
        const v = videoRef.value
        if (v && v.duration) {
          v.currentTime = (action.value / 100) * v.duration
          deps.writeProgress()
        }
        break
      }
      case 'sub-offset': {
        const next = Math.round((state.subOffset.value + action.value) * 10) / 10
        state.subOffset.value = next
        break
      }
      case 'mute':
        deps.onToggleMute()
        break
      case 'fullscreen':
        deps.onToggleFullscreen()
        break
      case 'subs':
        deps.toggleMenu('subs')
        break
      case 'pip':
        deps.onTogglePip()
        break
      case 'next-episode':
        // Shift+N (anytime) advances whenever a next episode exists; bare `n` is
        // prompt-scoped — only acts while the countdown card or the end chip is up.
        if (
          deps.anime_hasNextEp.value &&
          (action.anytime || deps.showNextEpisode.value || deps.showNextEpChip.value)
        ) {
          deps.goToNextEpisode()
        }
        break
    }
  }

  return { onKeydown, onDocumentPointerDown, onDocumentFocusIn }
}
