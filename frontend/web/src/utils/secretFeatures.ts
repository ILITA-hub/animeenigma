import type { RouteLocationRaw } from 'vue-router'
import { offlineDownloadsEnabled } from '@/offline/flag'
import { useStandaloneDisplay } from '@/pwa/standalone'
import { useProfileWallVisible } from '@/utils/profileWallGate'
import { useAuthStore } from '@/stores/auth'

/**
 * «Секретная фича» pool — hidden/legacy features that left the regular nav
 * (feedback 2026-07-04T07-37-57_tNeymik_manual). The Navbar tab opens a
 * random eligible entry; the routes themselves stay directly reachable.
 */
export interface SecretFeature {
  key: 'anidle' | 'status' | 'downloads' | 'showcase-editor'
  /** Navigation target for router.push. */
  to: RouteLocationRaw
  /** Plain path used to avoid re-rolling the page the user is already on. */
  path: string
  /** Evaluated at click time — Pinia stores and gates are live by then. */
  eligible: () => boolean
}

export const SECRET_FEATURES: SecretFeature[] = [
  { key: 'anidle', to: '/anidle', path: '/anidle', eligible: () => true },
  { key: 'status', to: '/status', path: '/status', eligible: () => true },
  {
    // In the installed PWA downloads keep their normal nav link; only the
    // browser view treats them as a secret.
    key: 'downloads',
    to: '/downloads',
    path: '/downloads',
    eligible: () => offlineDownloadsEnabled && !useStandaloneDisplay().value,
  },
  {
    // /profile redirects to /user/:publicId preserving the query;
    // Profile.vue opens the owner's showcase editor on ?showcase=edit.
    key: 'showcase-editor',
    to: { path: '/profile', query: { showcase: 'edit' } },
    path: '/profile',
    eligible: () => useAuthStore().isAuthenticated && useProfileWallVisible().value,
  },
]

let lastKey: SecretFeature['key'] | null = null

/**
 * Uniform random pick over eligible entries, skipping the current page and
 * the previous pick while alternatives remain. Never empty: anidle and
 * status are unconditional.
 */
export function pickSecretFeature(currentPath: string): SecretFeature {
  let pool = SECRET_FEATURES.filter((f) => f.eligible())
  const away = pool.filter((f) => f.path !== currentPath)
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
