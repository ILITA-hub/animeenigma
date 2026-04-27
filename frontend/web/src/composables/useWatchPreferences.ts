import { ref } from 'vue'
import { userApi } from '@/api/client'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

const CACHE_TTL = 24 * 60 * 60 * 1000 // 24 hours

export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)

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
    // Anon users now hit /api/preferences/resolve — the axios interceptor attaches
    // X-Anon-ID and the backend OptionalAuthMiddleware allows the call through.
    // Per CONTEXT Critical Finding 3: required for D-12 (per-anon-user override rate).
    if (available.length === 0) return

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
