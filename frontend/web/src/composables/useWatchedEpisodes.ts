import { ref, toValue, type MaybeRefOrGetter, type Ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

/**
 * Single source of the user's watched-episode count for an anime. Returns a
 * reactive `watchedUpTo` (episodes with number <= this are "watched") and a
 * `refresh()` the player calls on mount, on episode change, and after a
 * mark-watched. Per-user; never synced across Watch Together members.
 */
export function useWatchedEpisodes(animeId: MaybeRefOrGetter<string>): {
  watchedUpTo: Ref<number>
  refresh: () => Promise<void>
} {
  const watchedUpTo = ref(0)
  const auth = useAuthStore()

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
