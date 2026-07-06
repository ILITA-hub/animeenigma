import { computed, type ComputedRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import { useStandaloneDisplay, installHintKey } from '@/pwa/standalone'
import { offlineDownloadsEnabled } from './flag'

/** Setup-free view of the browser-tab-vs-installed-app decision: downloads
 *  enabled but running in a browser tab. Tracks reactively when read inside
 *  a computed/watcher (useStandaloneDisplay is a singleton ref). */
export function downloadsAppOnly(): boolean {
  return offlineDownloadsEnabled && !useStandaloneDisplay().value
}

/** Downloads as a normal nav item (header link, tab-bar tab) — only in the
 *  installed app; browser tabs reach /downloads via the secret-feature roll. */
export function downloadsNavVisible(): boolean {
  return offlineDownloadsEnabled && useStandaloneDisplay().value
}

/** Downloads are app-only (owner call 2026-07-04). Single owner of the
 *  browser-tab-vs-installed-app decision and of the "get the app" hint, so
 *  every download surface (player action row, episodes panel, card context
 *  menu) behaves identically. Call during setup(). */
export function useDownloadGate(): {
  /** True when download surfaces must point at the app instead of downloading. */
  appOnly: ComputedRef<boolean>
  showInstallHint: () => void
} {
  const toast = useToast()
  const { t } = useI18n()
  return {
    appOnly: computed(downloadsAppOnly),
    showInstallHint() {
      toast.push(t(installHintKey()), 'info', 6000)
    },
  }
}
