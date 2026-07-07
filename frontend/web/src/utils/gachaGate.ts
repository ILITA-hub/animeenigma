/**
 * Gacha visibility gate (RBAC-and-roulette P4 Task 2 cutover).
 *
 * Delegates to the runtime feature-visibility feed (`GET
 * /api/policy/features/mine`, composables/useFeatureVisible.ts) instead of
 * the retired VITE_GACHA_ADMIN_ONLY build flag. The feed's per-key
 * fail-open fallback (composables/useFeatureVisible.ts `DARKSHIP_FALLBACK_ADMIN`)
 * reproduces the old admin-only default while the feed hasn't loaded yet.
 *
 * Usage: `const gachaVisible = useGachaVisible()`
 */
import { useFeatureVisible } from '@/composables/useFeatureVisible'

/**
 * Returns a reactive boolean ref: whether the current user should see gacha
 * features (navbar item, balance chip, routes, Profile tab).
 */
export function useGachaVisible() {
  return useFeatureVisible('gacha')
}
