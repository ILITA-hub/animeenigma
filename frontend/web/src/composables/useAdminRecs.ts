import { ref, type Ref } from 'vue'
import { apiClient } from '@/api/client'

// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug page composable.
//
// Backend contract (services/player/internal/handler/admin_recs.go):
//   GET /api/admin/recs/{user_id} -> AdminRecsResponse
//   POST /api/admin/recs/{user_id}/recompute -> ForceRecomputeResponse

export interface AdminRecAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  episodes_count?: number
  status?: string
  year?: number
}

export interface AdminRecRow {
  rank: number
  anime: AdminRecAnime
  final: number
  breakdown: Record<string, number>
  weights: Record<string, number>
  top_contributor: string
  contributor_detail?: Record<string, unknown>
  pinned?: boolean
  pin_reason?: string
  pin_source?: string
  pin_seed_anime_id?: string
}

export interface FilteredOutEntry {
  anime_id: string
  reason: string
}

export interface AdminRecsResponse {
  recs: AdminRecRow[]
  filtered_out: FilteredOutEntry[]
  computed_at: string
  signal_versions: Record<string, string>
  user_id: string
}

export interface ForceRecomputeResponse {
  computed_at: string
  top_n_count: number
  latency_ms: number
}

export function useAdminRecs(userId: Ref<string>) {
  const rows = ref<AdminRecRow[]>([])
  const filteredOut = ref<FilteredOutEntry[]>([])
  const computedAt = ref<string>('')
  const signalVersions = ref<Record<string, string>>({})
  const isLoading = ref(false)
  const isRecomputing = ref(false)
  const error = ref<string | null>(null)
  const lastRecomputeLatencyMs = ref<number | null>(null)

  // Phase 12 / UA-096: map HTTP errors → friendly i18n keys.
  // The view templates can `$t(error)` directly when the value is a key
  // starting with `admin.errors.`; legacy `'403'` is preserved for
  // backwards compat with the existing red banner in AdminRecs.vue.
  function mapHttpError(e: unknown, fallback: string): string {
    const errObj = e as {
      response?: { status?: number }
      message?: string
      code?: string
    }
    const status = errObj?.response?.status
    const code = errObj?.code
    const msg = errObj?.message ?? ''
    // axios timeout: code 'ECONNABORTED', or message 'timeout of 0ms exceeded'
    if (
      code === 'ECONNABORTED' ||
      code === 'ETIMEDOUT' ||
      /timeout/i.test(msg) ||
      msg.toLowerCase().includes('aborted')
    ) {
      return 'admin.errors.timeout'
    }
    if (status === 401) return 'admin.errors.unauthorized'
    if (status === 403) return 'admin.errors.forbidden'
    if (status && status >= 500) return 'admin.errors.serverError'
    return msg || fallback
  }

  async function fetchRows(): Promise<void> {
    if (!userId.value) return
    isLoading.value = true
    error.value = null
    try {
      const res = await apiClient.get(`/admin/recs/${userId.value}`)
      // Backend wraps responses in { success, data } via httputil.OK.
      const env = (res.data?.data ?? res.data) as AdminRecsResponse
      rows.value = env.recs ?? []
      filteredOut.value = env.filtered_out ?? []
      computedAt.value = env.computed_at ?? ''
      signalVersions.value = env.signal_versions ?? {}
    } catch (e: unknown) {
      // Preserve legacy '403' for the existing red-banner template path.
      const status = (e as { response?: { status?: number } })?.response?.status
      if (status === 403) {
        error.value = '403'
      } else {
        error.value = mapHttpError(e, 'failed to load admin recs')
      }
      rows.value = []
      filteredOut.value = []
    } finally {
      isLoading.value = false
    }
  }

  async function recompute(): Promise<void> {
    if (!userId.value) return
    isRecomputing.value = true
    error.value = null
    try {
      const res = await apiClient.post(`/admin/recs/${userId.value}/recompute`)
      const env = (res.data?.data ?? res.data) as ForceRecomputeResponse
      lastRecomputeLatencyMs.value = env?.latency_ms ?? null
      await fetchRows()
    } catch (e: unknown) {
      const status = (e as { response?: { status?: number } })?.response?.status
      if (status === 403) {
        error.value = '403'
      } else {
        error.value = mapHttpError(e, 'recompute failed')
      }
    } finally {
      isRecomputing.value = false
    }
  }

  return {
    rows,
    filteredOut,
    computedAt,
    signalVersions,
    isLoading,
    isRecomputing,
    error,
    lastRecomputeLatencyMs,
    refresh: fetchRows,
    recompute,
  }
}
