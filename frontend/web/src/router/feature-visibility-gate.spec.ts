import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import router from './index'

/**
 * RBAC-and-roulette P4 Task 2 — router guard cutover.
 *
 * The `meta.gachaGated` / `meta.fanficGated` checks in router/index.ts used
 * to read the build-time GACHA_ADMIN_ONLY / FANFIC_ADMIN_ONLY consts; they
 * now `await` the feature-visibility store's `ready` promise and consult
 * `resolveVisible` (composables/useFeatureVisible.ts) instead. This exercises
 * the REAL router (with all its registered guards) via router.push(), unlike
 * admin-policy-route.spec.ts's router.resolve() (which does not run guards).
 *
 * featuresApi.getFeaturesMine is mocked per-test to control whether the feed
 * loads successfully (and with what `visible` set) or fails outright (the
 * D1 fail-open / dark-ship-admin-fallback path) — the store's `ready`
 * promise only resolves once `load()` settles, so the guard's `await
 * featureVisibility.ready` needs a controllable real (mocked) fetch, not
 * just a hand-set `loaded`/`visible` on the store.
 */
const { getFeaturesMine } = vi.hoisted(() => ({ getFeaturesMine: vi.fn() }))
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
  featuresApi: { getFeaturesMine },
}))

describe('router: gachaGated / fanficGated routes read the feature-visibility feed', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    getFeaturesMine.mockReset()
    // The router is a module-level singleton shared across tests in this
    // file. Navigating to the SAME path as the current route is a vue-router
    // "duplicate navigation" no-op (guards don't re-run) — reset to a neutral
    // route first so every test's push to /gacha or /fanfics is a genuine,
    // distinct navigation that re-runs the guard under test.
    await router.push('/')
  })

  function signIn(isAdmin: boolean) {
    const auth = useAuthStore()
    auth.token = 'faketoken'
    auth.user = {
      id: 'u1',
      username: isAdmin ? 'admin' : 'user',
      email: 'x@x.com',
      role: isAdmin ? 'admin' : 'user',
    } as never
  }

  it('feed loaded, gacha visible → non-admin reaches /gacha', async () => {
    getFeaturesMine.mockResolvedValue({ rouletteEnabled: false, visible: ['gacha'], roulette: [] })
    signIn(false)
    await router.push('/gacha')
    expect(router.currentRoute.value.name).toBe('gacha')
  })

  it('feed loaded, gacha NOT visible → non-admin is redirected home', async () => {
    getFeaturesMine.mockResolvedValue({ rouletteEnabled: false, visible: [], roulette: [] })
    signIn(false)
    await router.push('/gacha')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('feed fetch fails → dark-ship fallback lets an admin reach /gacha', async () => {
    getFeaturesMine.mockRejectedValue(new Error('network down'))
    signIn(true)
    await router.push('/gacha')
    expect(router.currentRoute.value.name).toBe('gacha')
  })

  it('feed fetch fails → dark-ship fallback redirects a non-admin home', async () => {
    getFeaturesMine.mockRejectedValue(new Error('network down'))
    signIn(false)
    await router.push('/gacha')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('feed loaded, fanfic visible → non-admin reaches /fanfics', async () => {
    getFeaturesMine.mockResolvedValue({ rouletteEnabled: false, visible: ['fanfic'], roulette: [] })
    signIn(false)
    await router.push('/fanfics')
    expect(router.currentRoute.value.name).toBe('fanfics')
  })

  it('feed loaded, fanfic NOT visible → non-admin is redirected home', async () => {
    getFeaturesMine.mockResolvedValue({ rouletteEnabled: false, visible: [], roulette: [] })
    signIn(false)
    await router.push('/fanfics')
    expect(router.currentRoute.value.name).toBe('home')
  })
})
