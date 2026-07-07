import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Prevent Axios from actually constructing during import of the auth store /
// featureVisibility store (both pull in @/api/client) — mirrors
// utils/__tests__/profileWallGate.spec.ts.
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
  featuresApi: { getFeaturesMine: vi.fn() },
}))

describe('useFeatureVisible', () => {
  beforeEach(() => {
    vi.resetModules()
    setActivePinia(createPinia())
  })

  it('feed loaded: visible.has(key) true for a present key (incl. a public key)', async () => {
    const { useFeatureVisible } = await import('@/composables/useFeatureVisible')
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const store = useFeatureVisibilityStore()
    store.loaded = true
    store.visible = new Set(['fanfic', 'anidle'])

    expect(useFeatureVisible('fanfic').value).toBe(true)
    expect(useFeatureVisible('anidle').value).toBe(true)
  })

  it('feed loaded: visible.has(key) false for an absent key (incl. a non-darkship public key)', async () => {
    const { useFeatureVisible } = await import('@/composables/useFeatureVisible')
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const store = useFeatureVisibilityStore()
    store.loaded = true
    store.visible = new Set(['anidle'])

    expect(useFeatureVisible('fanfic').value).toBe(false)
    expect(useFeatureVisible('themes').value).toBe(false)
  })

  it('feed NOT loaded: fanfic/gacha/profile-wall fall back to authStore.isAdmin (admin=true)', async () => {
    const { useFeatureVisible } = await import('@/composables/useFeatureVisible')
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const { useAuthStore } = await import('@/stores/auth')
    useFeatureVisibilityStore() // loaded defaults to false — nothing to load in this spec
    const auth = useAuthStore()
    auth.user = { role: 'admin', id: 'x', username: 'admin', email: 'admin@test.com' } as never

    expect(useFeatureVisible('fanfic').value).toBe(true)
    expect(useFeatureVisible('gacha').value).toBe(true)
    expect(useFeatureVisible('profile-wall').value).toBe(true)
  })

  it('feed NOT loaded: fanfic/gacha/profile-wall fall back to authStore.isAdmin (admin=false)', async () => {
    const { useFeatureVisible } = await import('@/composables/useFeatureVisible')
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const { useAuthStore } = await import('@/stores/auth')
    useFeatureVisibilityStore()
    const auth = useAuthStore()
    auth.user = { role: 'user', id: 'x', username: 'plain', email: 'plain@test.com' } as never

    expect(useFeatureVisible('fanfic').value).toBe(false)
    expect(useFeatureVisible('gacha').value).toBe(false)
    expect(useFeatureVisible('profile-wall').value).toBe(false)
  })

  it('feed NOT loaded: a public key (anidle) is true regardless of admin status', async () => {
    const { useFeatureVisible } = await import('@/composables/useFeatureVisible')
    const { useFeatureVisibilityStore } = await import('@/stores/featureVisibility')
    const { useAuthStore } = await import('@/stores/auth')
    useFeatureVisibilityStore()
    const auth = useAuthStore()
    auth.user = { role: 'user', id: 'x', username: 'plain', email: 'plain@test.com' } as never

    expect(useFeatureVisible('anidle').value).toBe(true)
  })
})
