import { ref, onMounted, watch } from 'vue'
import { userApi } from '@/api/client'
import { createFetchCache } from '@/composables/createFetchCache'
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

// Module-level cache — SPA route revisits within the TTL reuse the shared
// items instead of refetching (2026-07-04 trace: home→anime→home re-issued
// the identical request 12s later). 30s TTL is lossless: the player's
// heartbeat saves every 30s (useWatchTracking SAVE_INTERVAL), so the backend
// can't have meaningfully fresher data inside that window anyway.
const items = ref<ContinueWatchingItem[]>([])
const cache = createFetchCache(30 * 1000)
let cachedLimit = 0

export function useContinueWatching(limit = 10) {
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const auth = useAuthStore()

  async function fetchItems(force = false) {
    // Anonymous users skip the fetch entirely — the endpoint is JWT-
    // protected and would return 401.
    if (!auth.token) {
      items.value = []
      cache.invalidate()
      return
    }
    if (!force && cachedLimit === limit && cache.isFresh()) return
    return cache.share(async () => {
      isLoading.value = true
      error.value = null
      try {
        const res = await userApi.getContinueWatching(limit)
        const data = (res.data?.data ?? res.data) as ContinueWatchingItem[]
        items.value = Array.isArray(data) ? data : []
        cache.markFresh()
        cachedLimit = limit
      } catch (e) {
        error.value = e instanceof Error ? e.message : 'failed to load continue-watching'
        items.value = []
        cache.invalidate()
      } finally {
        isLoading.value = false
      }
    })
  }

  onMounted(fetchItems)

  // Re-fetch on auth transitions (login from anonymous, logout to anonymous).
  // Mirrors the pattern in useRecs.ts so the row appears/disappears without
  // a hard reload when the user signs in or signs out.
  //
  // IN-03 (Phase 8): dropped the prior `oldToken !== undefined` guard. The
  // watch source is auth.token, which is initialized to
  // localStorage.getItem('token') (string | null), never undefined. Without
  // immediate:true on this watcher the callback only fires on change, so
  // newToken !== oldToken is already guaranteed; both halves of the prior
  // condition were defensive no-ops.
  watch(
    () => auth.token,
    () => {
      // Force past the cache — a token change means the cached items belong
      // to a different identity (or to anonymous).
      fetchItems(true)
    },
  )

  return { items, isLoading, error, refresh: () => fetchItems(true) }
}
