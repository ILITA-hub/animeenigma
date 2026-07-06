/**
 * Fanfic engine visibility gate (spec 2026-07-06).
 *
 * VITE_FANFIC_ADMIN_ONLY defaults to TRUE (unset = true = admin-only dark-ship),
 * mirroring GACHA_ADMIN_ONLY in utils/gachaGate.ts. Flip to 'false' to expose
 * the feature to every authenticated (non-guest) user.
 *
 * Usage: `const fanficVisible = useFanficVisible()`
 */
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** True ⟹ only admins see the fanfic engine; false ⟹ every authenticated user. */
export const FANFIC_ADMIN_ONLY =
  (import.meta.env.VITE_FANFIC_ADMIN_ONLY as string | undefined) !== 'false'

/**
 * Returns a reactive boolean ref: whether the current user should see fanfic
 * engine features (navbar item, /fanfics route, generate/library UI).
 */
export function useFanficVisible() {
  const authStore = useAuthStore()
  return computed(() => {
    if (FANFIC_ADMIN_ONLY) return authStore.isAdmin
    return authStore.isAuthenticated
  })
}
