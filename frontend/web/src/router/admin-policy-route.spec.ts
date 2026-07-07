import { describe, it, expect } from 'vitest'
import router from './index'

// RBAC-and-roulette P3 (Task 8): the old secret-feature roulette page was
// absorbed into /admin/policy. This locks in the redirect + the new route's
// admin guard so a future edit can't silently drop either. Uses
// router.resolve() (route-matching only — does NOT run navigation guards,
// so no Pinia/i18n bootstrapping is needed here).
describe('router: /admin/policy + /admin/secret-features redirect', () => {
  it('resolves /admin/secret-features to a static redirect targeting /admin/policy', () => {
    const resolved = router.resolve('/admin/secret-features')
    expect(resolved.matched).toHaveLength(1)
    expect(resolved.matched[0].redirect).toBe('/admin/policy')
  })

  it('maps /admin/policy to the admin-gated AdminPolicy route', () => {
    const resolved = router.resolve('/admin/policy')
    expect(resolved.name).toBe('admin-policy')
    expect(resolved.meta.requiresAuth).toBe(true)
    expect(resolved.meta.requiresAdmin).toBe(true)
    expect(resolved.meta.titleKey).toBe('admin.policy.title')
  })

  it('no longer registers a standalone AdminSecretFeatures route', () => {
    const allNames = router.getRoutes().map((r) => r.name)
    expect(allNames).not.toContain('admin-secret-features')
  })
})
