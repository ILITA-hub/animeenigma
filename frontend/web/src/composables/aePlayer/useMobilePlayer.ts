import { ref, type Ref } from 'vue'

// Mobile-player environment as reactive refs. 680px mirrors the player CSS
// breakpoint (PlayerControlBar/AePlayer media queries); pointer:coarse gates
// touch-only behaviors (tap-to-toggle chrome, double-tap seek).
// Singleton: the MQL listeners live for the app's lifetime — one per query,
// shared across every player mount, so mounts never leak extra listeners.

interface MobilePlayerEnv {
  isMobile: Ref<boolean>
  isCoarse: Ref<boolean>
}

let cached: MobilePlayerEnv | null = null

function watchMedia(query: string): Ref<boolean> {
  const matches = ref(false)
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return matches
  const mql = window.matchMedia(query)
  matches.value = mql.matches
  mql.addEventListener?.('change', (e) => {
    matches.value = e.matches
  })
  return matches
}

export function useMobilePlayer(): MobilePlayerEnv {
  if (!cached) {
    cached = {
      isMobile: watchMedia('(max-width: 680px)'),
      isCoarse: watchMedia('(pointer: coarse)'),
    }
  }
  return cached
}

export function _resetMobilePlayerForTests(): void {
  cached = null
}
