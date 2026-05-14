import { apiClient } from './client'

export interface ApiSession {
  id: string
  user_agent: string
  ip: string
  created_at: string
  last_seen_at: string
  expires_at: string
  is_current: boolean
}

export const sessionsApi = {
  async list(): Promise<ApiSession[]> {
    const res = await apiClient.get('/auth/sessions')
    return res.data?.data ?? res.data ?? []
  },

  async revoke(id: string): Promise<void> {
    await apiClient.delete(`/auth/sessions/${encodeURIComponent(id)}`)
  },

  async revokeOthers(): Promise<number> {
    const res = await apiClient.post('/auth/sessions/revoke-others')
    return Number((res.data?.data ?? res.data)?.revoked ?? 0)
  },
}
