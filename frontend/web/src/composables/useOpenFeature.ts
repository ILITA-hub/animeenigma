import { useRouter } from 'vue-router'

/**
 * RBAC-and-roulette policy admin UI (Task 6) — contextual "open feature"
 * click helper for AdminPolicy.vue (Task 7)'s per-row `<a target="_blank">`
 * open-link.
 *
 * A plain browser tab opens the link natively (target="_blank"), so the
 * caller's anchor already does the right thing and this helper must do
 * NOTHING in that case — calling window.open() here would spawn a second,
 * redundant tab/instance.
 *
 * An installed standalone PWA has no address bar / tab strip, so a new-tab
 * anchor click is effectively swallowed (or opens a bare, chrome-less OS
 * window depending on platform). There we intercept the click and route
 * inside the existing app shell instead.
 *
 * `isStandalone` is computed once at composable setup — a fresh read per
 * call site is fine here since AdminPolicy.vue is a single always-mounted
 * admin view (no long-lived cross-page-load state to keep in sync), and it
 * keeps the value a plain boolean (easily assertable in tests) rather than
 * a reactive ref.
 */
export function useOpenFeature() {
  const router = useRouter()

  const isStandalone =
    window.matchMedia?.('(display-mode: standalone)')?.matches === true ||
    (navigator as unknown as { standalone?: boolean }).standalone === true

  function openFeature(e: MouseEvent, route: string): void {
    if (isStandalone) {
      e.preventDefault()
      router.push(route)
    }
    // Non-standalone: do nothing — the native <a target="_blank"> handles it.
  }

  return { isStandalone, openFeature }
}
