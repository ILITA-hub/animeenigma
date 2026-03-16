import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { userApi } from '@/api/client'

export const useWatchlistStore = defineStore('watchlist', () => {
  const statusEntries = ref<Array<{ anime_id: string; status: string }>>([])
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

  const getEntry = (animeId: string) => {
    return statusEntries.value.find(e => e.anime_id === animeId) || null
  }

  const invalidate = () => {
    statusLastFetched.value = 0
  }

  const clear = () => {
    statusEntries.value = []
    statusLastFetched.value = 0
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
  }
})
