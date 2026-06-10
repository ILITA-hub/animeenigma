/**
 * Gacha visibility gate.
 *
 * VITE_GACHA_ADMIN_ONLY defaults to TRUE (unset = true = admin-only dark-ship).
 * The bundled release flips it to 'false' to expose the feature to all users.
 *
 * Usage: `const gachaVisible = useGachaVisible()`
 */
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** True ⟹ only admins see gacha; false ⟹ every authenticated user sees it. */
export const GACHA_ADMIN_ONLY =
  (import.meta.env.VITE_GACHA_ADMIN_ONLY as string | undefined) !== 'false'

/**
 * Returns a reactive boolean ref: whether the current user should see gacha
 * features (navbar item, balance chip, routes, Profile tab).
 */
export function useGachaVisible() {
  const authStore = useAuthStore()
  return computed(() => {
    if (GACHA_ADMIN_ONLY) return authStore.isAdmin
    return authStore.isAuthenticated
  })
}
