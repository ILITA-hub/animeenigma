import { ref, onMounted, watch } from 'vue'
import { apiClient } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

// Phase 10: anonymous trending row composable.
// Phase 11: also handles auth-state transitions so the row payload swaps
// between "Trending now" (anonymous) and "Up Next for you" (logged-in)
// without requiring a hard reload.
//
// Backend contract (services/player/internal/handler/recs.go):
//   GET /api/users/recs -> { success, data: { recs, generated_at, cache_hit, total, row_label_key } }
//
// row_label_key is the i18n key the row title uses — Phase 10 returns
// "recs.trending" for anonymous, Phase 11 returns "recs.upNext" for
// logged-in callers. We expose it via the composable so Home.vue can render
// `$t(rowLabelKey)` without hard-coding the key.

export interface RecAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  episodes_count?: number
  year?: number
  status?: string
}

export interface RecItem {
  anime: RecAnime
  final: number
  pinned: boolean
  rank: number
  // Phase 13 (REC-UX-03) — present only when pinned===true. The server
  // renders pin_reason with the seed name interpolated as legacy English text.
  // UX-09 (Phase 3) adds pin_reason_key + pin_reason_data so the frontend can
  // render via $t() with locale-aware copy; consumers should prefer the key
  // path and fall back to pin_reason (raw English) when the key is absent.
  pin_reason?: string
  pin_reason_key?: string
  pin_reason_data?: Record<string, unknown>
  pin_seed_anime_id?: string
  pin_source?: 'local' | 'shikimori_similar' | 'score_5_fallback'
  // Phase 14 (REC-EVAL-01) — click-time signal_id surfaced by the backend
  // so the frontend can tag rec_click events without a separate fetch.
  // Empty for pinned items (frontend uses the literal "s6_pin" instead).
  top_contributor?: string
}

interface RecsEnvelope {
  recs: RecItem[]
  generated_at: string
  cache_hit: boolean
  total: number
  row_label_key: string
}

export function useRecs() {
  const recs = ref<RecItem[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const generatedAt = ref<string | null>(null)
  const rowLabelKey = ref<string>('recs.trending')
  const cacheHit = ref(false)

  async function fetchRecs() {
    isLoading.value = true
    error.value = null
    try {
      // apiClient is the shared axios instance (handles base URL, auth header
      // attachment, refresh-on-401). It uses VITE_API_URL or '/api' so the
      // path here is relative to that.
      const res = await apiClient.get('/users/recs')
      // Backend wraps every response in { success, data } via httputil.OK,
      // so unwrap data and be lenient if a future change inlines it.
      const env = (res.data?.data ?? res.data) as RecsEnvelope
      recs.value = env.recs ?? []
      generatedAt.value = env.generated_at ?? null
      rowLabelKey.value = env.row_label_key ?? 'recs.trending'
      cacheHit.value = !!env.cache_hit
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'failed to load recommendations'
      recs.value = []
    } finally {
      isLoading.value = false
    }
  }

  onMounted(fetchRecs)

  // Phase 11 (REC-UX-01): re-fetch when auth state transitions so the row
  // payload swaps between "Trending now" (anonymous) and "Up Next for you"
  // (logged-in) without a hard reload. Watching `token` (the source of
  // truth for "is the next request authenticated?") rather than the `user`
  // ref because `user` can lag during refresh-cookie warmups.
  const auth = useAuthStore()
  watch(
    () => auth.token,
    (newToken, oldToken) => {
      // Only refetch on a real transition; the watcher fires once on mount
      // with `oldToken === undefined` which would double-trigger after the
      // onMounted initial fetch.
      if (newToken !== oldToken && oldToken !== undefined) {
        fetchRecs()
      }
    },
  )

  return { recs, isLoading, error, generatedAt, rowLabelKey, cacheHit, refresh: fetchRecs }
}
