import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('analytics identity', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('anon id is stable then rotates on reset', async () => {
    const { getAnonId, resetAnon } = await import('../identity')
    const a = getAnonId()
    expect(getAnonId()).toBe(a)
    resetAnon()
    expect(getAnonId()).not.toBe(a)
  })

  it('user id persists and clears', async () => {
    const { getUserId, setUserId, clearUserId } = await import('../identity')
    expect(getUserId()).toBeNull()
    setUserId('u1')
    expect(getUserId()).toBe('u1')
    clearUserId()
    expect(getUserId()).toBeNull()
  })
})
