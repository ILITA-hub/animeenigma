import { ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

const CACHE_TTL = 24 * 60 * 60 * 1000 // 24 hours

export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)
  const authStore = useAuthStore()

  // Try cached result first
  const cacheKey = `pref:${animeId}`
  const cached = localStorage.getItem(cacheKey)
  if (cached) {
    try {
      const { data, timestamp } = JSON.parse(cached)
      if (Date.now() - timestamp < CACHE_TTL) {
        resolvedCombo.value = data
      }
    } catch { /* ignore corrupt cache */ }
  }

  async function resolve(available: WatchCombo[]) {
    if (!authStore.isAuthenticated || available.length === 0) return

    isLoading.value = true
    try {
      const { data } = await userApi.resolvePreference(animeId, available)
      resolvedCombo.value = data.resolved
      // Cache the result
      localStorage.setItem(cacheKey, JSON.stringify({
        data: data.resolved,
        timestamp: Date.now()
      }))
    } catch (err) {
      console.error('Failed to resolve preference:', err)
    } finally {
      isLoading.value = false
    }
  }

  return { resolvedCombo, isLoading, resolve }
}
