import type { RouteLocationRaw } from 'vue-router'
import { useFeatureVisibilityStore } from '@/stores/featureVisibility'

/**
 * «Секретная фича» pool — hidden/legacy features surfaced only through the
 * footer roulette (feedback 2026-07-04T07-37-57_tNeymik_manual). The routes
 * themselves stay directly reachable.
 *
 * This file is the key→route/label registry only. Pool MEMBERSHIP is fully
 * server-resolved (RBAC-and-roulette P4 Task 3): the policy service computes
 * `mine.roulette[]` per user (audience match + per-key roulette flag) via
 * `GET /api/policy/features/mine`, loaded once into
 * `stores/featureVisibility.ts`. {@link pickSecretFeature} just filters this
 * roster down to whatever keys the feed says are in the pool for the current
 * user — no more client-side `eligible()` checks, and no more
 * `applySecretFeatureAdminState` admin overlay (the admin surface is
 * AdminPolicy.vue, which edits the same policy flags this feed reads).
 */
export interface SecretFeature {
  key:
    | 'anidle'
    | 'status'
    | 'themes'
    | 'game'
    | 'gacha'
    | 'fanfic'
    | 'downloads'
    | 'showcase-editor'
    | 'my-feedback'
    | 'tips'
    | 'following'
  /** Navigation target for router.push (also the admin "direct link"). */
  to: RouteLocationRaw
  /** i18n key for the human label shown on the admin management page. */
  labelKey: string
}

export const SECRET_FEATURES: SecretFeature[] = [
  { key: 'anidle', to: '/anidle', labelKey: 'admin.secretFeatures.feature.anidle' },
  { key: 'status', to: '/status', labelKey: 'admin.secretFeatures.feature.status' },
  // OP/ED ratings and game rooms — public pages with no nav chrome; the
  // roulette is their only surfaced entry point (always reachable directly).
  { key: 'themes', to: '/themes', labelKey: 'admin.secretFeatures.feature.themes' },
  { key: 'game', to: '/game', labelKey: 'admin.secretFeatures.feature.game' },
  {
    // Gacha «Лудка» — pool membership is server-resolved (policy `gacha` flag's
    // roulette bit + audience match), not a client-side visibility mirror.
    key: 'gacha',
    to: '/gacha',
    labelKey: 'admin.secretFeatures.feature.gacha',
  },
  {
    // Fanfic engine — moved here from the header nav; the /fanfics route
    // stays directly reachable. Pool membership server-resolved same as gacha.
    key: 'fanfic',
    to: '/fanfics',
    labelKey: 'admin.secretFeatures.feature.fanfic',
  },
  {
    // In the installed PWA downloads keep their normal nav link too; pool
    // membership (whether this key rolls at all) is now server-resolved.
    key: 'downloads',
    to: '/downloads',
    labelKey: 'admin.secretFeatures.feature.downloads',
  },
  {
    // /profile redirects to /user/:publicId preserving the query;
    // Profile.vue opens the owner's showcase editor on ?showcase=edit.
    // Known mismatch: '/profile' never equals the post-redirect /user/:id
    // path, so the current-page exclusion skips this entry — rolling it
    // while on your own profile just opens the editor in place. Intended.
    key: 'showcase-editor',
    to: { path: '/profile', query: { showcase: 'edit' } },
    labelKey: 'admin.secretFeatures.feature.showcaseEditor',
  },
  {
    // "My feedback" archive — the footer link was removed (redundant with the
    // Feedback menu's "View mine"), so the roulette is now a surfaced entry
    // point.
    key: 'my-feedback',
    to: '/my-feedback',
    labelKey: 'admin.secretFeatures.feature.myFeedback',
  },
  {
    // Secret tips & hotkeys page — deliberately nav-less; discovered via the
    // roulette or the global F1 hotkey (App.vue + utils/globalHotkeys.ts).
    key: 'tips',
    to: '/tips',
    labelKey: 'admin.secretFeatures.feature.tips',
  },
  {
    key: 'following',
    to: '/following',
    labelKey: 'admin.secretFeatures.feature.following',
  },
]

/** Path used to avoid re-rolling the page the user is already on. */
const pathOf = (to: RouteLocationRaw): string | undefined =>
  typeof to === 'string' ? to : (to as { path?: string }).path

/** Human-readable direct-link path (incl. query) for the admin table. */
export function secretFeatureDisplayPath(f: SecretFeature): string {
  if (typeof f.to === 'string') return f.to
  const loc = f.to as { path?: string; query?: Record<string, string> }
  const q = loc.query ? '?' + new URLSearchParams(loc.query).toString() : ''
  return (loc.path ?? '') + q
}

/**
 * Whether the roulette actually has anything to roll for the current user —
 * the intersection of the client registry ({@link SECRET_FEATURES}) and the
 * server-resolved `store.roulette` pool. `rouletteEnabled` alone fails open
 * (stays true) on a total policy-feed outage, but an empty `store.roulette`
 * makes {@link pickSecretFeature} always return null — this lets callers
 * (the App.vue footer button) hide instead of rendering a dead affordance.
 */
export function roulettePoolAvailable(): boolean {
  const store = useFeatureVisibilityStore()
  return SECRET_FEATURES.some((f) => store.roulette.includes(f.key))
}

/**
 * Whether a click on the footer roulette button is an "open in a new tab"
 * gesture — Cmd (macOS) / Ctrl (Windows/Linux) + primary click, or a middle
 * click (`button === 1`). These are exactly the gestures the browser applies
 * to a real `<a href>`; the roulette is a `<button>` (its target is random per
 * click, so there's no static href), so App.vue's handler replays the gesture
 * itself via `window.open`. A modifier on a non-primary button (e.g. Ctrl +
 * right-click) is not a new-tab gesture and stays false.
 */
export function wantsNewTab(e: MouseEvent): boolean {
  if (e.button === 1) return true
  return e.button === 0 && (e.metaKey || e.ctrlKey)
}

let lastKey: SecretFeature['key'] | null = null

/**
 * Uniform random pick over the server-resolved pool (`store.roulette`),
 * skipping the current page and the previous pick while alternatives remain.
 * Returns null when the pool is empty (e.g. nothing is roulette-enabled for
 * this user) so callers can no-op instead of crashing.
 */
export function pickSecretFeature(currentPath: string): SecretFeature | null {
  const store = useFeatureVisibilityStore()
  let pool = SECRET_FEATURES.filter((f) => store.roulette.includes(f.key))
  if (pool.length === 0) return null
  const away = pool.filter((f) => pathOf(f.to) !== currentPath)
  if (away.length > 0) pool = away
  const fresh = pool.filter((f) => f.key !== lastKey)
  if (fresh.length > 0) pool = fresh
  const pick = pool[Math.floor(Math.random() * pool.length)]
  lastKey = pick.key
  return pick
}

export function _resetSecretFeatureForTests(): void {
  lastKey = null
}
