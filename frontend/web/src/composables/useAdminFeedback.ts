import { ref } from 'vue'
import { adminApi } from '@/api/client'
import { dayStartISO, dayEndISO } from '@/utils/time'
import type {
  FeedbackListItem,
  FeedbackListResponse,
  FeedbackDetail,
  FeedbackStatus,
} from '@/types/feedback'

// Admin feedback browser composable.
//
// Backend contract (services/player/internal/handler/admin_reports.go):
//   GET   /api/admin/reports?category=&status=&type=&page=&page_size=
//   GET   /api/admin/reports/{id}
//   PATCH /api/admin/reports/{id}/status   body {status}
//
// Mirrors useAdminRecs.ts: responses are wrapped in { success, data } via
// httputil.OK, so we unwrap `res.data?.data ?? res.data`. 403 maps to the
// literal '403' for the shared red-banner template path.

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

function mapErr(e: unknown): string {
  const obj = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  if (obj?.response?.status === 403) return '403'
  return obj?.response?.data?.error?.message || obj?.message || 'admin.feedback.errorGeneric'
}

export function useAdminFeedback() {
  const items = ref<FeedbackListItem[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(50)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  // 'all' is the sentinel for "no filter" — reka-ui's SelectItem forbids an
  // empty-string value, so the "All …" options use 'all' and we normalize it
  // away before hitting the API.
  const filterCategory = ref('all')
  const filterStatus = ref('all')
  const filterType = ref('all')
  // Free-text username filter (case-insensitive substring match, server-side).
  const filterUsername = ref('')
  // Submitted-at window (YYYY-MM-DD from <input type=date>); converted to
  // local-day RFC3339 bounds before hitting the API. Same date in both = one day.
  const filterDateFrom = ref('')
  const filterDateTo = ref('')

  const detail = ref<FeedbackDetail | null>(null)
  const isDetailLoading = ref(false)
  const detailError = ref<string | null>(null)

  async function refresh(): Promise<void> {
    isLoading.value = true
    error.value = null
    try {
      const norm = (v: string) => (v && v !== 'all' ? v : undefined)
      const res = await adminApi.listReports({
        category: norm(filterCategory.value),
        status: norm(filterStatus.value),
        type: norm(filterType.value),
        username: filterUsername.value.trim() || undefined,
        from: dayStartISO(filterDateFrom.value),
        to: dayEndISO(filterDateTo.value),
        page: page.value,
        page_size: pageSize.value,
      })
      const env = unwrap<FeedbackListResponse>(res.data)
      items.value = env.items ?? []
      total.value = env.total ?? 0
      page.value = env.page ?? 1
      pageSize.value = env.page_size ?? pageSize.value
    } catch (e: unknown) {
      error.value = mapErr(e)
      items.value = []
      total.value = 0
    } finally {
      isLoading.value = false
    }
  }

  // applyFilters resets to page 1 and reloads — call from filter @change.
  function applyFilters(): Promise<void> {
    page.value = 1
    return refresh()
  }

  function setPage(p: number): Promise<void> {
    if (p < 1) return Promise.resolve()
    page.value = p
    return refresh()
  }

  async function openDetail(id: string): Promise<void> {
    isDetailLoading.value = true
    detailError.value = null
    detail.value = null
    try {
      const res = await adminApi.getReport(id)
      detail.value = unwrap<FeedbackDetail>(res.data)
    } catch (e: unknown) {
      detailError.value = mapErr(e)
    } finally {
      isDetailLoading.value = false
    }
  }

  function closeDetail(): void {
    detail.value = null
    detailError.value = null
  }

  // setStatus optimistically updates the row + open detail, rolling back on error.
  async function setStatus(id: string, status: FeedbackStatus): Promise<void> {
    const row = items.value.find((i) => i.id === id)
    const prev = row?.status
    if (row) row.status = status
    if (detail.value && detail.value.id === id) detail.value.status = status
    try {
      await adminApi.setReportStatus(id, status)
    } catch (e: unknown) {
      if (row && prev) row.status = prev
      error.value = mapErr(e)
    }
  }

  return {
    items,
    total,
    page,
    pageSize,
    isLoading,
    error,
    filterCategory,
    filterStatus,
    filterType,
    filterUsername,
    filterDateFrom,
    filterDateTo,
    detail,
    isDetailLoading,
    detailError,
    refresh,
    applyFilters,
    setPage,
    openDetail,
    closeDetail,
    setStatus,
  }
}
