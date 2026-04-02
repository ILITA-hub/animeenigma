import { ref } from 'vue'
import { reviewApi } from '@/api/client'

interface SiteRating {
  anime_id: string
  average_score: number
  total_reviews: number
}

// Module-level session cache to avoid re-fetching across page navigations
const ratingCache = new Map<string, SiteRating>()

export function useSiteRatings() {
  const ratings = ref<Record<string, SiteRating>>({})
  const loading = ref(false)

  async function fetchRatings(ids: string[]) {
    if (!ids.length) return

    const uncached = ids.filter(id => !ratingCache.has(id))

    if (uncached.length > 0) {
      loading.value = true
      try {
        const resp = await reviewApi.getBatchRatings(uncached)
        const data = (resp.data ?? resp) as SiteRating[] | { ratings: SiteRating[] }
        const list = Array.isArray(data) ? data : (data as { ratings: SiteRating[] }).ratings ?? []
        for (const r of list) {
          ratingCache.set(r.anime_id, r)
        }
      } catch (e) {
        console.warn('Failed to fetch batch ratings:', e)
      } finally {
        loading.value = false
      }
    }

    // Build result for the requested IDs
    const result: Record<string, SiteRating> = {}
    for (const id of ids) {
      const cached = ratingCache.get(id)
      if (cached && cached.total_reviews > 0) {
        result[id] = cached
      }
    }
    ratings.value = result
  }

  return { ratings, loading, fetchRatings }
}
