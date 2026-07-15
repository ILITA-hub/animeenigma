import { type ComputedRef, type Ref } from 'vue'
import { mapKeyToAction } from '@/composables/aePlayer/playerHotkeys'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { MenuKind } from '@/composables/aePlayer/usePlayerMenus'

// ── Keyboard shortcuts ────────────────────────────────────────────────────────
// Listen on window but only act when the pointer is over the player or focus is
// inside it — so space/arrows control THIS player without hijacking the page.
// (The window listener itself is registered by the component's lifecycle hooks.)

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

  function playerIsActive(): boolean {
    if (isPointerInside.value) return true
    const root = rootRef.value
    return !!(root && document.activeElement && root.contains(document.activeElement))
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

  return { onKeydown }
}
