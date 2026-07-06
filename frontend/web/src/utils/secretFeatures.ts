import type { RouteLocationRaw } from 'vue-router'
import { downloadsAppOnly } from '@/offline/downloadGate'
import { useProfileWallVisible } from '@/utils/profileWallGate'
import { useAuthStore } from '@/stores/auth'

/**
 * «Секретная фича» pool — hidden/legacy features that left the regular nav
 * (feedback 2026-07-04T07-37-57_tNeymik_manual). The Navbar tab opens a
 * random eligible entry; the routes themselves stay directly reachable.
 */
export interface SecretFeature {
  key: 'anidle' | 'status' | 'themes' | 'game' | 'downloads' | 'showcase-editor'
  /** Navigation target for router.push. */
  to: RouteLocationRaw
  /** Evaluated at click time — Pinia stores and gates are live by then. */
  eligible: () => boolean
}

export const SECRET_FEATURES: SecretFeature[] = [
  { key: 'anidle', to: '/anidle', eligible: () => true },
  { key: 'status', to: '/status', eligible: () => true },
  // OP/ED ratings and game rooms — public pages with no nav chrome; the
  // roulette is their only surfaced entry point (always reachable directly).
  { key: 'themes', to: '/themes', eligible: () => true },
  { key: 'game', to: '/game', eligible: () => true },
  {
    // In the installed PWA downloads keep their normal nav link; only the
    // browser view treats them as a secret.
    key: 'downloads',
    to: '/downloads',
    eligible: downloadsAppOnly,
  },
  {
    // /profile redirects to /user/:publicId preserving the query;
    // Profile.vue opens the owner's showcase editor on ?showcase=edit.
    // Known mismatch: '/profile' never equals the post-redirect /user/:id
    // path, so the current-page exclusion skips this entry — rolling it
    // while on your own profile just opens the editor in place. Intended.
    key: 'showcase-editor',
    to: { path: '/profile', query: { showcase: 'edit' } },
    eligible: () => useAuthStore().isAuthenticated && useProfileWallVisible().value,
  },
]

/** Path used to avoid re-rolling the page the user is already on. */
const pathOf = (to: RouteLocationRaw): string | undefined =>
  typeof to === 'string' ? to : (to as { path?: string }).path

let lastKey: SecretFeature['key'] | null = null

/**
 * Uniform random pick over eligible entries, skipping the current page and
 * the previous pick while alternatives remain. Never empty: anidle and
 * status are unconditional.
 */
export function pickSecretFeature(currentPath: string): SecretFeature {
  let pool = SECRET_FEATURES.filter((f) => f.eligible())
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
