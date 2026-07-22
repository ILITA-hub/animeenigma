import { describe, expect, it } from 'vitest'
import router from './index'

describe('router: hidden Zundamon voice lab', () => {
  it('maps /zundamon to a public, non-admin route', () => {
    const resolved = router.resolve('/zundamon')

    expect(resolved.name).toBe('zundamon-tts')
    expect(resolved.meta.requiresAuth).toBeUndefined()
    expect(resolved.meta.requiresAdmin).toBeUndefined()
    expect(resolved.meta.titleKey).toBe('zundamon.title')
  })
})
