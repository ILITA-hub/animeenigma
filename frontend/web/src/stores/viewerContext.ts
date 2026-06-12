import { defineStore } from 'pinia'
import { ref } from 'vue'
import { animeApi } from '@/api/client'

/**
 * Per-anime viewer context — the aggregate payload behind
 * GET /api/anime/{id}/viewer-context (page-fetch optimization 2026-06-11).
 *
 * One request replaces the anime page's separate rating / watchers-count /
 * progress / watchlist-entry / my-review / saved-combo fetches. The store
 * holds the context for the CURRENTLY VIEWED anime only (navigating to a
 * different anime replaces it) and single-flights concurrent loads so
 * Anime.vue and UnifiedPlayer share one network call on mount.
 */

export interface ViewerProgressRow {
  episode_number?: number
  progress?: number
  duration?: number
  completed?: boolean
}

export interface ViewerWatchlistEntry {
  anime_id: string
  status: string
  episodes?: number
  score?: number
  rewatch_count?: number
  is_rewatching?: boolean
}

export interface ViewerCombo {
  player: string
  language: string
  watch_type: string
  translation_id?: string
  translation_title?: string
}

export interface ViewerContextData {
  rating: { anime_id: string; average_score: number; total_reviews: number } | null
  watchers_count: number
  progress: ViewerProgressRow[] | null
  watchlist_entry: ViewerWatchlistEntry | null
  my_review: { id: string; score: number; review_text?: string } | null
  combo: ViewerCombo | null
}

export const useViewerContextStore = defineStore('viewerContext', () => {
  const animeId = ref<string | null>(null)
  const data = ref<ViewerContextData | null>(null)
  const loading = ref(false)
  let inFlight: Promise<ViewerContextData | null> | null = null
  // Remembered per current anime so forced refreshes (mutations) keep the
  // mal_{id} legacy-entry fallback without every caller re-supplying it.
  let lastMalId: string | undefined

  /**
   * Load the context for `id`. Same-anime concurrent callers share one
   * in-flight request; a repeat call for the already-loaded anime returns the
   * cached payload unless `force` (used after mutations: review submitted,
   * episode marked watched, watchlist status changed).
   */
  async function load(id: string, force = false, malId?: string | number): Promise<ViewerContextData | null> {
    if (!id) return null
    if (!force && animeId.value === id) {
      if (data.value) return data.value
      if (inFlight) return inFlight
    }

    if (animeId.value !== id) lastMalId = undefined
    if (malId !== undefined && malId !== null && String(malId)) lastMalId = String(malId)

    animeId.value = id
    data.value = null
    loading.value = true
    const request = animeApi
      .getViewerContext(id, lastMalId)
      .then((res) => {
        const payload = (res.data?.data ?? res.data ?? null) as ViewerContextData | null
        // Guard against a stale response landing after the user navigated on.
        if (animeId.value === id) data.value = payload
        return payload
      })
      .catch(() => {
        // Best-effort: consumers fall back to their legacy per-endpoint
        // fetches when the context is unavailable.
        return null
      })
      .finally(() => {
        if (animeId.value === id) {
          loading.value = false
          inFlight = null
        }
      })
    inFlight = request
    return request
  }

  /** Cached context for `id`, or null when a different anime is loaded. */
  function forAnime(id: string): ViewerContextData | null {
    return animeId.value === id ? data.value : null
  }

  /**
   * Like forAnime, but waits for an in-flight load of the same anime instead
   * of reporting null. Deep-link consumers (player progress / watched-episodes
   * on `?episode=N`) mount while the aggregate request is still in the air —
   * a synchronous forAnime check there used to fall back to the very network
   * calls the aggregate exists to replace. Resolves null when nothing is
   * loaded or loading for `id`.
   */
  function whenLoaded(id: string): Promise<ViewerContextData | null> {
    if (animeId.value !== id) return Promise.resolve(null)
    if (data.value) return Promise.resolve(data.value)
    return inFlight ?? Promise.resolve(null)
  }

  function reset() {
    animeId.value = null
    data.value = null
    inFlight = null
    loading.value = false
  }

  return { animeId, data, loading, load, forAnime, whenLoaded, reset }
})
