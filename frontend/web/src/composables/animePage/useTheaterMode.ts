import { ref, watch, onMounted, onBeforeUnmount } from 'vue'

/**
 * Phase 11 / UX-23 — Theater Mode (extracted from Anime.vue).
 *
 * State persists via localStorage so a reload keeps the user's choice.
 * `body.theater-mode` drives the CSS rules at the bottom of Anime.vue —
 * the navbar and the rest of the page STAY visible; theater only widens
 * the player wrapper to full-bleed and caps its height. onBeforeUnmount
 * removes the body class so leaving /anime/:id never leaves the class
 * (and its now-inert-elsewhere selectors) stuck on, e.g. on /downloads or
 * /watch-together.
 */
export function useTheaterMode() {
  const theaterMode = ref<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('theaterMode') === '1',
  )

  function setTheater(on: boolean) {
    theaterMode.value = on
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('theaterMode', on ? '1' : '0')
    }
  }

  function applyBodyTheaterClass(on: boolean) {
    if (typeof document === 'undefined') return
    document.body.classList.toggle('theater-mode', on)
  }

  function onTheaterEscape(e: KeyboardEvent) {
    if (e.key === 'Escape' && theaterMode.value) {
      setTheater(false)
    }
  }

  // Apply persisted theater-mode state + bind ESC.
  onMounted(() => {
    applyBodyTheaterClass(theaterMode.value)
    document.addEventListener('keydown', onTheaterEscape)
  })

  // Cleanup — strips the body class so navigating away from /anime/:id never
  // leaves `body.theater-mode` set on other routes (e.g. /downloads,
  // /watch-together), where the theater CSS selectors would otherwise still
  // (harmlessly, since they target Anime.vue-only elements) match nothing
  // but the stale class itself would remain a footgun for future CSS.
  onBeforeUnmount(() => {
    applyBodyTheaterClass(false)
    document.removeEventListener('keydown', onTheaterEscape)
  })

  // React to programmatic state changes (toggle button click, ESC).
  watch(theaterMode, (on) => applyBodyTheaterClass(on))

  return { theaterMode, setTheater }
}
