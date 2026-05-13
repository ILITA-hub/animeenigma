import { ref, onMounted, watch } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

// Phase 8 (UX-15 / UA-061): Continue-Watching row for the logged-in Home
// view. Anonymous users are never even fetched — the composable returns
// `items: []` and the row is hidden by the parent. We still mount the
// auth watcher so a login transition triggers the first fetch without
// a page reload.
//
// Backend contract (services/player/internal/handler/progress.go):
//   GET /api/users/continue-watching?limit=N
//   -> { success, data: ContinueWatchingItem[] }

export interface ContinueWatchingAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  episodes_count?: number
}

export interface ContinueWatchingItem {
  anime: ContinueWatchingAnime
  episode_number: number
  progress: number
  duration: number
  last_watched_at: string
  dropped_off_at?: number | null
}

export function useContinueWatching(limit = 10) {
  const items = ref<ContinueWatchingItem[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const auth = useAuthStore()

  async function fetchItems() {
    // Anonymous users skip the fetch entirely — the endpoint is JWT-
    // protected and would return 401.
    if (!auth.token) {
      items.value = []
      return
    }
    isLoading.value = true
    error.value = null
    try {
      const res = await userApi.getContinueWatching(limit)
      const data = (res.data?.data ?? res.data) as ContinueWatchingItem[]
      items.value = Array.isArray(data) ? data : []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'failed to load continue-watching'
      items.value = []
    } finally {
      isLoading.value = false
    }
  }

  onMounted(fetchItems)

  // Re-fetch on auth transitions (login from anonymous, logout to anonymous).
  // Mirrors the pattern in useRecs.ts so the row appears/disappears without
  // a hard reload when the user signs in or signs out.
  watch(
    () => auth.token,
    (newToken, oldToken) => {
      if (newToken !== oldToken && oldToken !== undefined) {
        fetchItems()
      }
    },
  )

  return { items, isLoading, error, refresh: fetchItems }
}
