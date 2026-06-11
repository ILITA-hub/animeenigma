import { ref, toValue, type MaybeRefOrGetter, type Ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore } from '@/stores/viewerContext'

/**
 * Single source of the user's watched-episode count for an anime. Returns a
 * reactive `watchedUpTo` (episodes with number <= this are "watched") and a
 * `refresh()` the player calls on mount, on episode change, and after a
 * mark-watched. Per-user; never synced across Watch Together members.
 *
 * Page-fetch optimization (2026-06-11): the FIRST refresh per anime consumes
 * the already-loaded viewer-context aggregate (when present) instead of
 * fetching /users/watchlist/{id} — every player mount used to fire that
 * request. Subsequent refreshes (after mark-watched mutations) go to the
 * network as before, so post-mutation values are never stale.
 */
export function useWatchedEpisodes(animeId: MaybeRefOrGetter<string>): {
  watchedUpTo: Ref<number>
  refresh: () => Promise<void>
} {
  const watchedUpTo = ref(0)
  const auth = useAuthStore()
  let prefetchConsumedFor: string | null = null

  async function refresh(): Promise<void> {
    if (!auth.isAuthenticated) {
      watchedUpTo.value = 0
      return
    }
    const id = toValue(animeId)
    if (!id) {
      watchedUpTo.value = 0
      return
    }
    if (prefetchConsumedFor !== id) {
      prefetchConsumedFor = id
      // whenLoaded (not forAnime): on deep-link mounts the aggregate request
      // is often still in flight — await it instead of falling back to the
      // network call it exists to replace.
      const ctx = await useViewerContextStore().whenLoaded(id)
      if (ctx) {
        watchedUpTo.value = Number(ctx.watchlist_entry?.episodes) || 0
        return
      }
    }
    try {
      const res = await userApi.getWatchlistEntry(id)
      const entry = res.data?.data ?? res.data
      watchedUpTo.value = Number(entry?.episodes) || 0
    } catch {
      // No entry / not in list / network — treat as none watched.
      watchedUpTo.value = 0
    }
  }

  return { watchedUpTo, refresh }
}
