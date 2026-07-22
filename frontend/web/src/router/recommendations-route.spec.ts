import { describe, expect, it } from 'vitest'
import router from './index'

describe('router: hidden recommendations page', () => {
  it('maps /recs to an authenticated, non-admin route', () => {
    const resolved = router.resolve('/recs')

    expect(resolved.name).toBe('recommendations')
    expect(resolved.meta.requiresAuth).toBe(true)
    expect(resolved.meta.requiresAdmin).toBeUndefined()
    expect(resolved.meta.titleKey).toBe('recs.pageTitle')
  })
})
