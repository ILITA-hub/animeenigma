/**
 * Profile showcase ("стена") visibility gate.
 *
 * VITE_PROFILE_WALL_ADMIN_ONLY defaults to TRUE (unset = admin-only dark-ship).
 * The bundled release flips it to 'false' to expose the feature to all users.
 * Mirror of utils/gachaGate.ts.
 */
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** True ⟹ only admins see the showcase; false ⟹ every authenticated user does. */
export const PROFILE_WALL_ADMIN_ONLY =
  (import.meta.env.VITE_PROFILE_WALL_ADMIN_ONLY as string | undefined) !== 'false'

/**
 * Reactive boolean: whether the current user should see the profile showcase
 * feature (read + the owner editor entry). When released this means "any
 * authenticated user"; during dark-ship it means "admins only".
 */
export function useProfileWallVisible() {
  const authStore = useAuthStore()
  return computed(() => {
    if (PROFILE_WALL_ADMIN_ONLY) return authStore.isAdmin
    return authStore.isAuthenticated
  })
}
