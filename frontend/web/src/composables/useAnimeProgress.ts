import { ref, watch, type Ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

// Phase 9 (UX-16): bulk per-card anime-progress fetch. Anonymous users skip
// the fetch entirely — the endpoint is JWT-protected and would return 401.
// The composable batches all IDs passed in `ids` into a single network call
// and exposes a Map<animeId, ProgressEntry> the card can look up by ID.
//
// Backend contract (services/player/internal/handler/progress.go::GetBulkProgress):
//   GET /api/users/anime-progress?ids=a,b,c   (max 50)
//   -> { success, data: { [animeId]: ProgressEntry } }
//
// Backend omits animes the user has no progress on; the frontend treats
// absence-in-map as "no badge".

export interface ProgressEntry {
  latest_episode: number
  episodes_count: number
  episodes_aired: number
  completed: boolean
  dropped: boolean
}

export function useAnimeProgress(ids: Ref<string[]>, debounceMs = 200) {
  const progressMap = ref<Map<string, ProgressEntry>>(new Map())
  const loading = ref(false)
  const error = ref<string | null>(null)
  const auth = useAuthStore()

  let debounceHandle: ReturnType<typeof setTimeout> | null = null

  async function fetchProgress(currentIds: string[]) {
    if (!auth.token || currentIds.length === 0) {
      progressMap.value = new Map()
      return
    }
    // Cap at 50 to match backend; if the caller passes more we slice the
    // visible window and accept that the trailing cards render without a
    // badge until the user paginates.
    const trimmed = currentIds.slice(0, 50)
    loading.value = true
    error.value = null
    try {
      const res = await userApi.getAnimeProgress(trimmed)
      const data = (res.data?.data ?? res.data) as Record<string, ProgressEntry>
      const next = new Map<string, ProgressEntry>()
      if (data && typeof data === 'object') {
        for (const [k, v] of Object.entries(data)) {
          next.set(k, v)
        }
      }
      progressMap.value = next
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'failed to load anime-progress'
      progressMap.value = new Map()
    } finally {
      loading.value = false
    }
  }

  // Debounced watcher — re-fetch when the ids list changes or the auth
  // token transitions (login/logout). immediate:true triggers an initial
  // fetch on mount; fetchProgress fast-paths empty/anonymous cases.
  watch(
    [ids, () => auth.token],
    ([newIds]) => {
      if (debounceHandle) clearTimeout(debounceHandle)
      const snapshot = [...newIds]
      debounceHandle = setTimeout(() => fetchProgress(snapshot), debounceMs)
    },
    { immediate: true },
  )

  return { progressMap, loading, error, refresh: () => fetchProgress(ids.value) }
}
