/**
 * Fanfic engine visibility gate (RBAC-and-roulette P4 Task 2 cutover).
 *
 * Delegates to the runtime feature-visibility feed (`GET
 * /api/policy/features/mine`, composables/useFeatureVisible.ts) instead of
 * the retired VITE_FANFIC_ADMIN_ONLY build flag. The feed's per-key
 * fail-open fallback (composables/useFeatureVisible.ts `DARKSHIP_FALLBACK_ADMIN`)
 * reproduces the old admin-only default while the feed hasn't loaded yet.
 *
 * Usage: `const fanficVisible = useFanficVisible()`
 */
import type { LocationQuery } from 'vue-router'
import { useFeatureVisible } from '@/composables/useFeatureVisible'

/**
 * Returns a reactive boolean ref: whether the current user should see fanfic
 * engine features (navbar item, /fanfics route, generate/library UI).
 */
export function useFanficVisible() {
  return useFeatureVisible('fanfic')
}

/**
 * The daily-fanfic deep link contract, shared by its producer
 * (DailyFanficCard's "Читать" CTA) and consumers (the router guard's
 * gate bypass, FanficsView's onMounted): one place to rename the param.
 * The daily reader is public by design — see the router guard comment.
 */
export const DAILY_FANFIC_LINK = '/fanfics?daily=1'

/** Whether a route query is the daily-fanfic deep link (`?daily=1`). */
export function isDailyFanficQuery(query: LocationQuery): boolean {
  return query.daily === '1'
}
