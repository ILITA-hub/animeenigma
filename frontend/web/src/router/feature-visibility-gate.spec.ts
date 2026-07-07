import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
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

  // RBAC-and-roulette P5 Task 4 (B3) — cold-load stall bound.
  //
  // A prior version of the guard did a bare `await featureVisibility.ready`,
  // which only resolves once `load()` settles (success OR failure — see
  // stores/featureVisibility.ts). If the policy service is simply hanging
  // (not erroring outright) on a first-navigation deep-link, that `await`
  // would block the whole navigation up to axios's 30s timeout before
  // falling open. These two tests mock `getFeaturesMine` with a promise that
  // NEVER settles (simulating a hung feed) and assert the navigation still
  // completes — via the guard's `Promise.race([ready, timeout(2500)])` — well
  // before that 30s ceiling, landing on the same D1 failSafe outcome
  // (admin allowed / non-admin redirected home) as the outright-failure case
  // above. Fake timers let the test advance exactly 2.5s of virtual time
  // instead of waiting on (or racing) real wall-clock time; if the guard
  // regressed to an unbounded await, `router.push` would never resolve and
  // this test would time out instead of asserting the wrong route.
  describe('cold-load timeout: feed fetch hangs and never settles', () => {
    afterEach(() => {
      vi.useRealTimers()
    })

    it('an admin still reaches /gacha once the 2.5s cold-load timeout elapses', async () => {
      vi.useFakeTimers()
      getFeaturesMine.mockReturnValue(new Promise(() => {}))
      signIn(true)
      const pushPromise = router.push('/gacha')
      await vi.advanceTimersByTimeAsync(2500)
      await pushPromise
      expect(router.currentRoute.value.name).toBe('gacha')
    })

    it('a non-admin is redirected home once the 2.5s cold-load timeout elapses', async () => {
      vi.useFakeTimers()
      getFeaturesMine.mockReturnValue(new Promise(() => {}))
      signIn(false)
      const pushPromise = router.push('/gacha')
      await vi.advanceTimersByTimeAsync(2500)
      await pushPromise
      expect(router.currentRoute.value.name).toBe('home')
    })
  })
})
