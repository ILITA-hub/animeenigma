import { ref, watch, onMounted, onBeforeUnmount } from 'vue'

/**
 * Phase 11 / UX-23 — Theater Mode (extracted from Anime.vue).
 *
 * State persists via localStorage so a reload keeps the user's choice.
 * `body.theater-mode` drives the CSS rules at the bottom of Anime.vue that
 * hide .navbar-root + .non-player-content and widen the player wrapper.
 * MANDATORY cleanup: onBeforeUnmount removes the body class so leaving
 * /anime/:id never strands the rest of the app with a hidden navbar.
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

  // MANDATORY theater-mode cleanup — strips the body class so navigating away
  // from /anime/:id never leaves the global navbar / non-player sections
  // hidden everywhere else.
  onBeforeUnmount(() => {
    applyBodyTheaterClass(false)
    document.removeEventListener('keydown', onTheaterEscape)
  })

  // React to programmatic state changes (toggle button click, ESC).
  watch(theaterMode, (on) => applyBodyTheaterClass(on))

  return { theaterMode, setTheater }
}
