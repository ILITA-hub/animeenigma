/** Initial-navigation-only offline landing: opening the app with no network
 *  goes straight to /downloads — the only fully usable surface offline.
 *  In-app navigation while offline is never hijacked. */
export function shouldRedirectToDownloads(opts: {
  isInitialNav: boolean
  online: boolean
  enabled: boolean
  toPath: string
}): boolean {
  return opts.isInitialNav && !opts.online && opts.enabled && opts.toPath !== '/downloads'
}
