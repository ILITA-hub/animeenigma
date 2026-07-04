import { ref, onMounted } from 'vue'
import { apiClient } from '@/api/client'
import type { SpotlightCard, SpotlightResponse } from '@/types/spotlight'

/**
 * useSpotlight — fetches the HeroSpotlightBlock card payload from
 * `GET /api/home/spotlight` and exposes reactive refs for the consuming
 * `HeroSpotlightBlock.vue` component (introduced in Plan 02-03).
 *
 * Returns:
 *   - cards   Ref<SpotlightCard[]>  Empty until the first fetch resolves;
 *                                    stays empty on any error so the block
 *                                    self-hides (UI-SPEC §State Contract).
 *   - loading Ref<boolean>          true between mount and first response.
 *   - error   Ref<Error | null>     populated on 5xx / network errors;
 *                                    `null` on success and on defensive
 *                                    null-cards path (no throw → no error).
 *   - refresh () => Promise<void>   Manual re-fetch trigger.
 *
 * Fetches on mount with a module-level 3-minute cache (SPA route revisits
 * within the TTL reuse the shared cards; `refresh()` always refetches).
 * Phase 3 will add an auth-state watcher (re-fetch on
 * login/logout transitions, mirroring useContinueWatching.ts:72) and a
 * 30-second `useIntervalFn` poll for live cards (now_watching,
 * latest_news refresh). Both extensions are additive — destructured
 * names below (`cards, loading, error, refresh`) are the locked API.
 *
 * Error contract: ONE `console.warn('[spotlight] fetch failed', e)` on
 * any catch path so observability/auditing can grep for it without
 * surfacing a user-visible toast. The block self-hides silently.
 */
// Module-level state — shared across mounts so SPA route revisits within the
// TTL don't refetch (2026-07-04 trace: home→anime→home re-issued the identical
// /home/spotlight request 11s after the first). TTL mirrors stores/home.ts.
const CACHE_TTL = 3 * 60 * 1000
const cards = ref<SpotlightCard[]>([])
let cachedAt = 0
let inFlight: Promise<void> | null = null

export function useSpotlight() {
  const loading = ref(cachedAt === 0)
  const error = ref<Error | null>(null)

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await apiClient.get<SpotlightResponse | { data: SpotlightResponse }>(
        '/home/spotlight',
      )
      // Defensive envelope unwrap — some catalog endpoints wrap responses
      // in {success, data:{...}}, others return the raw payload. Mirrors
      // useContinueWatching.ts:50 — `(res.data?.data ?? res.data)`.
      const body =
        ((res.data as { data?: SpotlightResponse })?.data as SpotlightResponse | undefined) ??
        (res.data as SpotlightResponse)
      // Array.isArray guard — backend may return `cards: null` on partial
      // failure or test fixtures; the block must self-hide rather than
      // crash the render.
      cards.value = Array.isArray(body?.cards) ? body.cards : []
      cachedAt = Date.now()
    } catch (e) {
      // Single warn for observability per UI-SPEC §State Contract. No
      // toast / banner — silent self-hide.
      // eslint-disable-next-line no-console
      console.warn('[spotlight] fetch failed', e)
      error.value = e instanceof Error ? e : new Error('spotlight fetch failed')
      cards.value = []
    } finally {
      loading.value = false
    }
  }

  // Fresh-cache mounts render the shared cards instantly; concurrent mounts
  // share one in-flight request instead of racing duplicates.
  function refreshIfStale(): Promise<void> {
    if (cards.value.length > 0 && Date.now() - cachedAt < CACHE_TTL) {
      loading.value = false
      return Promise.resolve()
    }
    if (!inFlight) {
      inFlight = refresh().finally(() => {
        inFlight = null
      })
    } else {
      // Track the shared request so this consumer's spinner clears with it.
      void inFlight.finally(() => {
        loading.value = false
      })
    }
    return inFlight
  }

  onMounted(refreshIfStale)

  return { cards, loading, error, refresh }
}
