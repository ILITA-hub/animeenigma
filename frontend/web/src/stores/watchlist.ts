import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

type WatchlistEntry = {
  anime_id: string
  status: string
  score?: number
  episodes?: number
}

export const useWatchlistStore = defineStore('watchlist', () => {
  const statusEntries = ref<WatchlistEntry[]>([])
  const statusLastFetched = ref<number>(0)
  const statusLoading = ref(false)
  const STATUS_CACHE_TTL = 2 * 60 * 1000 // 2 minutes

  const watchlistMap = computed(() => {
    const map = new Map<string, string>()
    for (const entry of statusEntries.value) {
      if (entry.anime_id && entry.status) {
        map.set(entry.anime_id, entry.status)
      }
    }
    return map
  })

  const isStatusFresh = () => Date.now() - statusLastFetched.value < STATUS_CACHE_TTL

  const fetchStatuses = async (force = false) => {
    // Anonymous users must no-op here. Without this gate, Home.vue fires
    // GET /users/watchlist/statuses → 401 → response interceptor calls
    // /auth/refresh → 401 → window.location='/' redirect — and on reload
    // the same call fires again, locking the home page in a redirect loop
    // (most visible on mobile where ITP invalidates the refresh cookie).
    const authStore = useAuthStore()
    if (!authStore.isAuthenticated) return

    if (!force && isStatusFresh() && statusEntries.value.length > 0) return
    if (statusLoading.value) return

    statusLoading.value = true
    try {
      const response = await userApi.getWatchlistStatuses()
      statusEntries.value = response.data?.data || response.data || []
      statusLastFetched.value = Date.now()
    } catch {
      // Silently fail — status map is non-critical
    } finally {
      statusLoading.value = false
    }
  }

  // Backward compat: fetchWatchlist now fetches statuses
  const fetchWatchlist = async (force = false) => {
    return fetchStatuses(force)
  }

  const getStatus = (animeId: string): string | null => {
    return watchlistMap.value.get(animeId) || null
  }

  const getEntry = (animeId: string): WatchlistEntry | null => {
    return statusEntries.value.find(e => e.anime_id === animeId) || null
  }

  const invalidate = () => {
    statusLastFetched.value = 0
  }

  const clear = () => {
    statusEntries.value = []
    statusLastFetched.value = 0
  }

  // ---------------------------------------------------------------------------
  // Phase 13 — Optimistic actions
  //
  // Each action mutates `statusEntries` immediately so consumers see the new
  // state before the network round-trip. On API failure we restore the prior
  // entry (or remove the just-pushed one) and re-throw so the caller can show
  // a toast. Concurrent edits are last-write-wins.
  // ---------------------------------------------------------------------------

  const findIndex = (animeId: string): number =>
    statusEntries.value.findIndex(e => e.anime_id === animeId)

  const setStatusOptimistic = async (animeId: string, newStatus: string): Promise<void> => {
    const idx = findIndex(animeId)
    const prior: WatchlistEntry | null = idx >= 0 ? { ...statusEntries.value[idx] } : null

    // Mutate locally first.
    if (idx >= 0) {
      statusEntries.value[idx] = { ...statusEntries.value[idx], status: newStatus }
    } else {
      statusEntries.value.push({ anime_id: animeId, status: newStatus })
    }

    try {
      if (prior === null) {
        await userApi.addToWatchlist(animeId, newStatus)
      } else {
        await userApi.updateWatchlistStatus(animeId, newStatus)
      }
    } catch (err) {
      // Rollback: restore prior state.
      const curIdx = findIndex(animeId)
      if (prior === null) {
        if (curIdx >= 0) statusEntries.value.splice(curIdx, 1)
      } else {
        if (curIdx >= 0) {
          statusEntries.value[curIdx] = prior
        } else {
          statusEntries.value.push(prior)
        }
      }
      throw err
    }
  }

  const setScoreOptimistic = async (animeId: string, newScore: number): Promise<void> => {
    const idx = findIndex(animeId)
    const prior: WatchlistEntry | null = idx >= 0 ? { ...statusEntries.value[idx] } : null

    // Edge case: scoring before adding. Add as 'completed' first.
    if (prior === null) {
      statusEntries.value.push({ anime_id: animeId, status: 'completed', score: newScore })
      try {
        await userApi.addToWatchlist(animeId, 'completed')
        await userApi.updateWatchlistEntry({
          anime_id: animeId,
          status: 'completed',
          score: newScore,
        })
      } catch (err) {
        const curIdx = findIndex(animeId)
        if (curIdx >= 0) statusEntries.value.splice(curIdx, 1)
        throw err
      }
      return
    }

    // Existing entry: update score in place.
    statusEntries.value[idx] = { ...statusEntries.value[idx], score: newScore }

    try {
      await userApi.updateWatchlistEntry({
        anime_id: animeId,
        status: prior.status,
        score: newScore,
      })
    } catch (err) {
      const curIdx = findIndex(animeId)
      if (curIdx >= 0) {
        statusEntries.value[curIdx] = prior
      } else {
        statusEntries.value.push(prior)
      }
      throw err
    }
  }

  const removeEntryOptimistic = async (animeId: string): Promise<void> => {
    const idx = findIndex(animeId)
    if (idx < 0) {
      // Nothing to remove locally; still issue API call in case server has it.
      try {
        await userApi.removeFromWatchlist(animeId)
      } catch (err) {
        throw err
      }
      return
    }

    const prior: WatchlistEntry = { ...statusEntries.value[idx] }
    statusEntries.value.splice(idx, 1)

    try {
      await userApi.removeFromWatchlist(animeId)
    } catch (err) {
      // Rollback: re-insert at original index (or push if list shifted).
      const restoreAt = Math.min(idx, statusEntries.value.length)
      statusEntries.value.splice(restoreAt, 0, prior)
      throw err
    }
  }

  const entries = statusEntries

  return {
    entries,
    loading: statusLoading,
    watchlistMap,
    fetchWatchlist,
    fetchStatuses,
    getStatus,
    getEntry,
    invalidate,
    clear,
    setStatusOptimistic,
    setScoreOptimistic,
    removeEntryOptimistic,
  }
})
