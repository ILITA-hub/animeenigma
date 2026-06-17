import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Prevent Axios from actually constructing during import of auth store
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
}))

describe('profileWallGate', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('hides for non-admin when admin-only (default)', async () => {
    vi.resetModules()
    const { useProfileWallVisible } = await import('@/utils/profileWallGate')
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    // isAdmin = user.value?.role === 'admin'; set role to non-admin so isAdmin=false
    auth.user = { role: 'user', id: 'x', username: 'test', email: 'test@test.com' } as never
    const visible = useProfileWallVisible()
    // PROFILE_WALL_ADMIN_ONLY defaults true (VITE_PROFILE_WALL_ADMIN_ONLY not set to 'false')
    // ⇒ non-admin user sees nothing
    expect(visible.value).toBe(false)
  })
})
