import { computed, type ComputedRef } from 'vue'
import { useStandaloneDisplay } from '@/pwa/standalone'
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

/** Downloads are app-only (owner call 2026-07-04; browser-tab surfaces fully
 *  hidden 2026-07-14). Single owner of the browser-tab-vs-installed-app
 *  decision, so every download surface (player action row, episodes panel,
 *  card context menu) behaves identically: a plain browser tab shows no
 *  download affordance at all. Call during setup(). */
export function useDownloadGate(): {
  /** True in a plain browser tab — download surfaces hide themselves entirely. */
  appOnly: ComputedRef<boolean>
} {
  return { appOnly: computed(downloadsAppOnly) }
}
