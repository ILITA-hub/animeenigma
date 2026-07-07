/**
 * RBAC-and-roulette P4 — per-feature visibility composable.
 *
 * Reads the boot-loaded `/api/policy/features/mine` feed (via
 * stores/featureVisibility.ts) to decide whether the current user should
 * see feature `key` (nav items, dark-ship routes, footer roulette pool
 * membership, etc.).
 *
 * Usage: `const fanficVisible = useFeatureVisible('fanfic')`
 */
import { computed, type ComputedRef } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useFeatureVisibilityStore } from '@/stores/featureVisibility'

/**
 * Dark-ship keys that fall back to admin-only when the feed hasn't loaded
 * yet (or a total policy outage) — spec D1. Every other key falls back to
 * visible, reproducing today's exact behavior without the
 * VITE_*_ADMIN_ONLY env dependency.
 */
export const DARKSHIP_FALLBACK_ADMIN = new Set(['fanfic', 'gacha', 'profile-wall'])

/**
 * The fail-open visibility decision, shared by `useFeatureVisible` (below,
 * reactive/setup-context) and the P4 Task 2 router guard (not a setup
 * context — reads the store instance + authStore.isAdmin directly). Kept as
 * a plain function so both call sites can't drift apart.
 *
 * `feed.loaded` false ⟹ per-key failSafe: dark-ship keys → `isAdmin`,
 * everything else → `true`. `feed.loaded` true ⟹ `feed.visible.has(key)`.
 */
export function resolveVisible(
  key: string,
  feed: { loaded: boolean; visible: Set<string> },
  isAdmin: boolean,
): boolean {
  if (feed.loaded) return feed.visible.has(key)
  return DARKSHIP_FALLBACK_ADMIN.has(key) ? isAdmin : true
}

/**
 * Reactive boolean: whether the current user should see feature `key`.
 */
export function useFeatureVisible(key: string): ComputedRef<boolean> {
  const store = useFeatureVisibilityStore()
  const authStore = useAuthStore()
  return computed(() =>
    resolveVisible(key, { loaded: store.loaded, visible: store.visible }, authStore.isAdmin),
  )
}
