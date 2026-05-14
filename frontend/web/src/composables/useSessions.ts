import { ref } from 'vue'
import { sessionsApi, type ApiSession } from '@/api/sessions'

export function useSessions() {
  const sessions = ref<ApiSession[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      sessions.value = await sessionsApi.list()
    } catch (e: unknown) {
      error.value = (e as Error)?.message ?? 'load_failed'
    } finally {
      loading.value = false
    }
  }

  async function revoke(id: string) {
    await sessionsApi.revoke(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
  }

  async function revokeOthers() {
    const n = await sessionsApi.revokeOthers()
    sessions.value = sessions.value.filter(s => s.is_current)
    return n
  }

  return { sessions, loading, error, refresh, revoke, revokeOthers }
}
