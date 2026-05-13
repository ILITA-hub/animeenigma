import { ref, onMounted, onBeforeUnmount } from 'vue'
import { apiClient } from '@/api/client'

// Phase 11 / UX-24 — System-status banner channel. Polls
// GET /api/system/status every `pollIntervalMs` (default 60s) and exposes
// the active incident list. Anonymous-safe — the gateway route is public
// and does not require a JWT. The frontend banner component (SystemStatusBanner.vue)
// renders nothing when the incident list is empty, so a permanent mount
// on Home is safe.
//
// Backend contract (services/gateway/internal/handler/system_status.go):
//   GET /api/system/status
//   -> { success, data: { incidents: Incident[] } }
//
// v0.1 always surfaces at most ONE incident sourced from the gateway env
// (SYSTEM_BANNER_ACTIVE + SYSTEM_BANNER_MESSAGE); future phases swap the
// backend for a real ops-pipeline read without breaking this contract.

export interface Incident {
  id: string
  severity: string
  title: string
  since: string
}

export function useSystemStatus(pollIntervalMs = 60000) {
  const incidents = ref<Incident[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function fetchStatus() {
    loading.value = true
    error.value = null
    try {
      const res = await apiClient.get('/system/status')
      // The gateway's httputil.OK wraps the payload as { success, data }.
      // Fall through to res.data directly when the wrapper is absent so we
      // stay tolerant of future shape changes.
      const data = (res.data?.data ?? res.data) as { incidents?: Incident[] }
      incidents.value = Array.isArray(data?.incidents) ? data.incidents : []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'failed to load system status'
      incidents.value = []
    } finally {
      loading.value = false
    }
  }

  onMounted(() => {
    fetchStatus()
    if (pollIntervalMs > 0) {
      timer = setInterval(fetchStatus, pollIntervalMs)
    }
  })

  onBeforeUnmount(() => {
    if (timer) clearInterval(timer)
    timer = null
  })

  return { incidents, loading, error, refresh: fetchStatus }
}
