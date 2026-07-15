import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { onClickOutside } from '@vueuse/core'

// ── Menu state (floating dropdowns / mobile bottom sheets) ───────────────────

export type MenuKind = 'source' | 'settings' | 'subs' | 'episodes' | null

export interface PlayerMenusDeps {
  openMenu: Ref<MenuKind>
  browseOpen: Ref<boolean>
  downloadDialogOpen: Ref<boolean>
  isMobile: ComputedRef<boolean> | Ref<boolean>
  nativeFsActive: Ref<boolean>
  videoRef: Ref<HTMLVideoElement | null>
}

export function usePlayerMenus(deps: PlayerMenusDeps) {
  const { openMenu, browseOpen, downloadDialogOpen, isMobile, nativeFsActive, videoRef } = deps

  // One floating dropdown is open at a time (mutually-exclusive v-if), so the
  // active element resolves from whichever menu `openMenu` selects.
  const sourceMenuEl = ref<HTMLElement | null>(null)
  const episodesMenuEl = ref<HTMLElement | null>(null)
  const settingsMenuEl = ref<HTMLElement | null>(null)
  const subsMenuEl = ref<HTMLElement | null>(null)
  const activeMenuEl = computed<HTMLElement | null>(() => {
    switch (openMenu.value) {
      case 'source': return sourceMenuEl.value
      case 'episodes': return episodesMenuEl.value
      case 'settings': return settingsMenuEl.value
      case 'subs': return subsMenuEl.value
      default: return null
    }
  })

  // Click-outside dismiss: a click anywhere outside the open dropdown closes it.
  // Ignore the trigger regions — the control bar (source/subs/settings pills) and
  // top bar (EP trigger) own their own toggle, so letting click-outside fire there
  // too would race the trigger and reopen-then-close. The <video> is also ignored
  // because onVideoClick handles it (and suppresses the play/pause side effect);
  // without this the pointerdown-phase click-outside would close the menu first,
  // leaving onVideoClick's click to fall through to togglePlay.
  onClickOutside(
    activeMenuEl,
    () => { if (openMenu.value !== null) closeMenus() },
    { ignore: ['.pl-controls', '.pl-top', videoRef] },
  )

  function toggleMenu(menu: MenuKind) {
    openMenu.value = openMenu.value === menu ? null : menu
    if (openMenu.value !== null) browseOpen.value = false
  }

  function closeMenus() {
    openMenu.value = null
    browseOpen.value = false
  }

  // Mobile sheets: teleport the floating menus to <body> and present them as
  // bottom sheets. Disabled inside NATIVE fullscreen — body children render
  // under the fullscreen element — where fixed positioning already fills the
  // fullscreen viewport correctly in place.
  const sheetTeleport = computed(() => isMobile.value && !nativeFsActive.value)
  const anySheetOpen = computed(() => openMenu.value !== null || browseOpen.value || downloadDialogOpen.value)

  function closeAllSheets() {
    closeMenus()
    downloadDialogOpen.value = false
  }

  return {
    sourceMenuEl,
    episodesMenuEl,
    settingsMenuEl,
    subsMenuEl,
    activeMenuEl,
    toggleMenu,
    closeMenus,
    sheetTeleport,
    anySheetOpen,
    closeAllSheets,
  }
}
