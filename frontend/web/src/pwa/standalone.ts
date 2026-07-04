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

/** i18n key for the "downloads live in the app" hint shown when a download
 *  surface is tapped from a plain browser tab — install steps differ on iOS
 *  (Share → Add to Home Screen) vs everything else (address-bar install). */
export function installHintKey(): string {
  const ios = typeof navigator !== 'undefined' && /iPad|iPhone|iPod/.test(navigator.userAgent)
  return ios ? 'downloads.installHintIos' : 'downloads.installHint'
}
