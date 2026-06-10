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
        // Backend shape: { success, data: { ratings: { [anime_id]: SiteRating } } } —
        // unwrap the envelope and accept either the id-keyed map or a flat array.
        const envelope = (resp.data ?? resp) as { data?: { ratings?: unknown }; ratings?: unknown }
        const ratingsField = envelope.data?.ratings ?? envelope.ratings ?? envelope
        const list: SiteRating[] = Array.isArray(ratingsField)
          ? ratingsField
          : ratingsField && typeof ratingsField === 'object'
            ? (Object.values(ratingsField) as SiteRating[])
            : []
        for (const r of list) {
          if (r && typeof r.anime_id === 'string') ratingCache.set(r.anime_id, r)
        }
        // Negative-cache misses so absent ratings aren't re-requested every page
        for (const id of uncached) {
          if (!ratingCache.has(id)) {
            ratingCache.set(id, { anime_id: id, average_score: 0, total_reviews: 0 })
          }
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
