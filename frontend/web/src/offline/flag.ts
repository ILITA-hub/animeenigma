/** Offline downloads UI gate. Default ON; set VITE_OFFLINE_DOWNLOADS_ENABLED=false
 *  at build time to yank all download surfaces without touching the SW. */
export const offlineDownloadsEnabled: boolean =
  import.meta.env.VITE_OFFLINE_DOWNLOADS_ENABLED !== 'false'

/** Runtime capability: downloads need a controlling SW to play back later. */
export function offlineRuntimeReady(): boolean {
  return offlineDownloadsEnabled && 'serviceWorker' in navigator && !!navigator.serviceWorker.controller
}
