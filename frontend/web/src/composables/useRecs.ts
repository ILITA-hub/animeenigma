import { ref, onMounted } from 'vue'
import { apiClient } from '@/api/client'

// Phase 10: anonymous trending row composable.
//
// Backend contract (services/player/internal/handler/recs.go):
//   GET /api/users/recs -> { success, data: { recs, generated_at, cache_hit, total, row_label_key } }
//
// row_label_key is the i18n key the row title uses ("recs.trending" in
// Phase 10; Phase 11 will branch to "recs.upNext" for logged-in users).
// We expose it via the composable so Home.vue can render `$t(rowLabelKey)`
// without hard-coding the key.

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

  return { recs, isLoading, error, generatedAt, rowLabelKey, cacheHit, refresh: fetchRecs }
}
