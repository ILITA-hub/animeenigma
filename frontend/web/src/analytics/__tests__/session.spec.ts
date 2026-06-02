import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('getSessionId', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('creates and persists a session id', async () => {
    const { getSessionId } = await import('../session')
    const now = Date.parse('2026-06-02T10:00:00Z')
    const id = getSessionId(now)
    expect(id).toBeTruthy()
    expect(getSessionId(now)).toBe(id) // stable within the window
  })

  it('rotates after 30 minutes of inactivity', async () => {
    const { getSessionId } = await import('../session')
    const t0 = Date.parse('2026-06-02T10:00:00Z')
    const id1 = getSessionId(t0)
    const id2 = getSessionId(t0 + 31 * 60 * 1000)
    expect(id2).not.toBe(id1)
  })

  it('keeps the session within the idle window', async () => {
    const { getSessionId } = await import('../session')
    const t0 = Date.parse('2026-06-02T10:00:00Z')
    const id1 = getSessionId(t0)
    const id2 = getSessionId(t0 + 20 * 60 * 1000)
    expect(id2).toBe(id1)
  })

  it('rotates on a new UTC day', async () => {
    const { getSessionId } = await import('../session')
    const id1 = getSessionId(Date.parse('2026-06-02T23:59:00Z'))
    const id2 = getSessionId(Date.parse('2026-06-03T00:05:00Z'))
    expect(id2).not.toBe(id1)
  })
})
