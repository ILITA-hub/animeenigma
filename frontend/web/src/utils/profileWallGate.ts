/**
 * Profile showcase ("стена") visibility gate (RBAC-and-roulette P4 Task 2
 * cutover).
 *
 * Delegates to the runtime feature-visibility feed (`GET
 * /api/policy/features/mine`, composables/useFeatureVisible.ts) instead of
 * the retired VITE_PROFILE_WALL_ADMIN_ONLY build flag. The feed's per-key
 * fail-open fallback (composables/useFeatureVisible.ts `DARKSHIP_FALLBACK_ADMIN`)
 * reproduces the old admin-only default while the feed hasn't loaded yet.
 * Mirror of utils/gachaGate.ts.
 */
import { useFeatureVisible } from '@/composables/useFeatureVisible'

/**
 * Reactive boolean: whether the current user should see the profile showcase
 * feature (read + the owner editor entry). When released this means "any
 * authenticated user"; during dark-ship it means "admins only".
 */
export function useProfileWallVisible() {
  return useFeatureVisible('profile-wall')
}
