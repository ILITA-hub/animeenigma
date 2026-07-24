/**
 * Reload detection via the Navigation Timing API.
 *
 * Used by the anime page to decide whether a `?episode=N` in the address bar
 * should jump the page down to the player. The param is written into the URL by
 * the player's url-sync while you watch, so a mid-watch reload (F5) would
 * otherwise re-trigger the jump — the "scroll-down on reload" bug (2026-07-24).
 *
 * The distinction we need is "this document was produced by a browser reload of
 * THIS exact URL", which is stricter than nav-type alone:
 *   - reload of the anime page  → type 'reload'  + name === current href → suppress
 *   - cold deep-link open        → type 'navigate'                        → scroll
 *   - in-app deep-link click      → loaded a different document (name mismatch) → scroll
 *   - back/forward               → type 'back_forward'                    → scroll
 * The name check is what saves the in-app case: after e.g. reloading the home
 * page then clicking into an anime, nav-type is still a stale 'reload', but the
 * reloaded document's URL (home) no longer matches the current anime href.
 */
export function isReloadOntoUrl(currentHref: string): boolean {
  if (typeof performance === 'undefined') return false
  const [nav] = (performance.getEntriesByType?.('navigation') ?? []) as PerformanceNavigationTiming[]
  return nav?.type === 'reload' && nav.name === currentHref
}

/** Convenience wrapper reading the live location — the real call site. */
export function isReloadOntoCurrentUrl(): boolean {
  if (typeof window === 'undefined') return false
  return isReloadOntoUrl(window.location.href)
}
