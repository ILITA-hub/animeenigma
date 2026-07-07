/**
 * Fanfic engine visibility gate (RBAC-and-roulette P4 Task 2 cutover).
 *
 * Delegates to the runtime feature-visibility feed (`GET
 * /api/policy/features/mine`, composables/useFeatureVisible.ts) instead of
 * the retired VITE_FANFIC_ADMIN_ONLY build flag. The feed's per-key
 * fail-open fallback (utils/useFeatureVisible.ts `DARKSHIP_FALLBACK_ADMIN`)
 * reproduces the old admin-only default while the feed hasn't loaded yet.
 *
 * Usage: `const fanficVisible = useFanficVisible()`
 */
import { useFeatureVisible } from '@/composables/useFeatureVisible'

/**
 * Returns a reactive boolean ref: whether the current user should see fanfic
 * engine features (navbar item, /fanfics route, generate/library UI).
 */
export function useFanficVisible() {
  return useFeatureVisible('fanfic')
}
