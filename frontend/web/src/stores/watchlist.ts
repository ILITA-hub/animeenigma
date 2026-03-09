import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { userApi } from '@/api/client'

export const useWatchlistStore = defineStore('watchlist', () => {
  const entries = ref<Array<{ anime_id: string; status: string; [key: string]: unknown }>>([])
  const lastFetched = ref<number>(0)
  const loading = ref(false)
  const CACHE_TTL = 2 * 60 * 1000 // 2 minutes

  const watchlistMap = computed(() => {
    const map = new Map<string, string>()
    for (const entry of entries.value) {
      if (entry.anime_id && entry.status) {
        map.set(entry.anime_id, entry.status)
      }
    }
    return map
  })

  const isFresh = () => Date.now() - lastFetched.value < CACHE_TTL

  const fetchWatchlist = async (force = false) => {
    if (!force && isFresh() && entries.value.length > 0) return
    if (loading.value) return

    loading.value = true
    try {
      const response = await userApi.getWatchlist()
      entries.value = response.data?.data || response.data || []
      lastFetched.value = Date.now()
    } catch {
      // Silently fail — watchlist is non-critical
    } finally {
      loading.value = false
    }
  }

  const getStatus = (animeId: string): string | null => {
    return watchlistMap.value.get(animeId) || null
  }

  const getEntry = (animeId: string) => {
    return entries.value.find(e => e.anime_id === animeId) || null
  }

  const invalidate = () => {
    lastFetched.value = 0
  }

  const clear = () => {
    entries.value = []
    lastFetched.value = 0
  }

  return {
    entries,
    loading,
    watchlistMap,
    fetchWatchlist,
    getStatus,
    getEntry,
    invalidate,
    clear,
  }
})
