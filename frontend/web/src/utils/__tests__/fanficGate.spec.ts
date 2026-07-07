import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Prevent Axios from actually constructing during import of auth store /
// featureVisibility store.
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
  featuresApi: { getFeaturesMine: vi.fn() },
}))

describe('fanficGate (RBAC-and-roulette P4 Task 2 — feed-driven)', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('delegates to useFeatureVisible("fanfic"): feed loaded + key present → visible', async () => {
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const { useFanficVisible } = await import('@/utils/fanficGate')
    const feed = useFeatureVisibilityStore()
    feed.loaded = true
    feed.visible = new Set(['fanfic'])

    const visible = useFanficVisible()
    expect(visible.value).toBe(true)
  })

  it('feed loaded + key absent → hidden, even for an admin', async () => {
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const { useAuthStore } = await import('@/stores/auth')
    const { useFanficVisible } = await import('@/utils/fanficGate')
    const feed = useFeatureVisibilityStore()
    feed.loaded = true
    feed.visible = new Set([])
    const auth = useAuthStore()
    auth.user = { id: 'a1', username: 'admin', email: 'a@a.com', role: 'admin' } as never

    const visible = useFanficVisible()
    expect(visible.value).toBe(false)
  })

  it('feed not loaded → fail-open fallback is admin-only (admin sees it)', async () => {
    const { useAuthStore } = await import('@/stores/auth')
    const { useFanficVisible } = await import('@/utils/fanficGate')
    const auth = useAuthStore()
    auth.user = { id: 'a1', username: 'admin', email: 'a@a.com', role: 'admin' } as never

    const visible = useFanficVisible()
    expect(visible.value).toBe(true)
  })

  it('feed not loaded → fail-open fallback hides it from a non-admin', async () => {
    const { useAuthStore } = await import('@/stores/auth')
    const { useFanficVisible } = await import('@/utils/fanficGate')
    const auth = useAuthStore()
    auth.user = { id: 'u1', username: 'user', email: 'u@u.com', role: 'user' } as never

    const visible = useFanficVisible()
    expect(visible.value).toBe(false)
  })
})
