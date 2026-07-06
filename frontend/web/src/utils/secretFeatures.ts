import type { RouteLocationRaw } from 'vue-router'
import { downloadsAppOnly } from '@/offline/downloadGate'
import { useProfileWallVisible } from '@/utils/profileWallGate'
import { useAuthStore } from '@/stores/auth'

/**
 * «Секретная фича» pool — hidden/legacy features surfaced only through the
 * footer roulette (feedback 2026-07-04T07-37-57_tNeymik_manual). The routes
 * themselves stay directly reachable.
 *
 * This file is the single source of truth for the pool + client-side
 * eligibility. Admin on/off overrides are layered on top via
 * {@link applySecretFeatureAdminState} (fetched from the backend by App.vue);
 * the admin management page (AdminSecretFeatures.vue) renders this roster.
 */
export interface SecretFeature {
  key: 'anidle' | 'status' | 'themes' | 'game' | 'downloads' | 'showcase-editor'
  /** Navigation target for router.push (also the admin "direct link"). */
  to: RouteLocationRaw
  /** i18n key for the human label shown on the admin management page. */
  labelKey: string
  /** Evaluated at click time — Pinia stores and gates are live by then. */
  eligible: () => boolean
}

export const SECRET_FEATURES: SecretFeature[] = [
  { key: 'anidle', to: '/anidle', labelKey: 'admin.secretFeatures.feature.anidle', eligible: () => true },
  { key: 'status', to: '/status', labelKey: 'admin.secretFeatures.feature.status', eligible: () => true },
  // OP/ED ratings and game rooms — public pages with no nav chrome; the
  // roulette is their only surfaced entry point (always reachable directly).
  { key: 'themes', to: '/themes', labelKey: 'admin.secretFeatures.feature.themes', eligible: () => true },
  { key: 'game', to: '/game', labelKey: 'admin.secretFeatures.feature.game', eligible: () => true },
  {
    // In the installed PWA downloads keep their normal nav link; only the
    // browser view treats them as a secret.
    key: 'downloads',
    to: '/downloads',
    labelKey: 'admin.secretFeatures.feature.downloads',
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
    labelKey: 'admin.secretFeatures.feature.showcaseEditor',
    eligible: () => useAuthStore().isAuthenticated && useProfileWallVisible().value,
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

// --- Admin override state (fail-open) --------------------------------------
// Set from the backend `/api/secret-features/state` feed by App.vue. Defaults
// mean "roulette on, nothing disabled" so a fetch failure = today's behavior.
let rouletteMasterEnabled = true
let adminDisabled = new Set<string>()

/** Apply the backend-resolved roulette state; null resets to the fail-open default. */
export function applySecretFeatureAdminState(
  state: { rouletteEnabled: boolean; disabledKeys: string[] } | null,
): void {
  if (!state) {
    rouletteMasterEnabled = true
    adminDisabled = new Set()
    return
  }
  rouletteMasterEnabled = state.rouletteEnabled
  adminDisabled = new Set(state.disabledKeys)
}

/** Master switch — whether the footer roulette button should be shown at all. */
export function isRouletteEnabled(): boolean {
  return rouletteMasterEnabled
}

let lastKey: SecretFeature['key'] | null = null

/**
 * Uniform random pick over eligible, admin-enabled entries, skipping the
 * current page and the previous pick while alternatives remain. Returns null
 * when nothing is eligible (e.g. an admin disabled every feature) so callers
 * can no-op instead of crashing.
 */
export function pickSecretFeature(currentPath: string): SecretFeature | null {
  let pool = SECRET_FEATURES.filter((f) => f.eligible() && !adminDisabled.has(f.key))
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
  rouletteMasterEnabled = true
  adminDisabled = new Set()
}
