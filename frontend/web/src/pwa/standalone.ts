import { ref, type Ref } from 'vue'

// Installed-PWA detection as a reactive ref. `display-mode: standalone` covers
// Android/desktop installs; iOS Safari exposes the legacy `navigator.standalone`
// boolean instead. Singleton: one MQL listener for the app's lifetime.

let cached: Ref<boolean> | null = null

export function useStandaloneDisplay(): Ref<boolean> {
  if (cached) return cached
  const standalone = ref(false)
  cached = standalone
  if (typeof window === 'undefined') return standalone
  const nav = navigator as Navigator & { standalone?: boolean }
  if (typeof window.matchMedia === 'function') {
    const mql = window.matchMedia('(display-mode: standalone)')
    standalone.value = mql.matches || nav.standalone === true
    mql.addEventListener?.('change', (e) => {
      standalone.value = e.matches || nav.standalone === true
    })
  } else {
    standalone.value = nav.standalone === true
  }
  return standalone
}

export function _resetStandaloneForTests(): void {
  cached = null
}
