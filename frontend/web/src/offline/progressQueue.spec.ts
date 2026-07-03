import 'fake-indexeddb/auto'
import { describe, it, expect, beforeEach } from 'vitest'
import { isAxiosError } from 'axios'
import { flushPendingProgress } from './progressQueue'
import { _resetDbForTests, enqueuePending } from './registry'

beforeEach(() => _resetDbForTests())

// queueProgress is a void fire-and-forget wrapper over enqueuePending (already
// covered in registry.spec.ts) — flush logic is what needs testing here, so
// seed the queue with awaited enqueuePending calls.
describe('progressQueue', () => {
  it('flushes queued payloads FIFO through the poster fn', async () => {
    await enqueuePending({ anime_id: 'a', episode_number: 1, progress: 100 })
    await enqueuePending({ anime_id: 'a', episode_number: 2, progress: 50 })
    const posted: unknown[] = []
    const ok = await flushPendingProgress(async (p) => { posted.push(p) })
    expect(ok).toBe(true)
    expect(posted).toHaveLength(2)
    expect((posted[0] as { episode_number: number }).episode_number).toBe(1)
  })
  it('keeps entries when posting fails', async () => {
    await enqueuePending({ anime_id: 'a', episode_number: 1, progress: 100 })
    expect(await flushPendingProgress(async () => { throw new Error('offline') })).toBe(false)
    const posted: unknown[] = []
    await flushPendingProgress(async (p) => { posted.push(p) })
    expect(posted).toHaveLength(1)
  })
  it('drops a non-retryable (400) payload and continues to the next entry', async () => {
    await enqueuePending({ anime_id: 'bad', episode_number: 1, progress: 100 })
    await enqueuePending({ anime_id: 'a', episode_number: 2, progress: 50 })
    const posted: unknown[] = []
    const badError = Object.assign(new Error('bad'), { isAxiosError: true, response: { status: 400 } })
    expect(isAxiosError(badError)).toBe(true) // sanity-check the axios-shaped fake is recognized
    const ok = await flushPendingProgress(async (p) => {
      if ((p as { anime_id: string }).anime_id === 'bad') throw badError
      posted.push(p)
    })
    expect(ok).toBe(true)
    expect(posted).toHaveLength(1)
    expect((posted[0] as { episode_number: number }).episode_number).toBe(2)
  })
  it('keeps entries when a network-style rejection occurs (no axios response)', async () => {
    await enqueuePending({ anime_id: 'a', episode_number: 1, progress: 100 })
    const ok = await flushPendingProgress(async () => { throw new Error('Network Error') })
    expect(ok).toBe(false)
    const posted: unknown[] = []
    await flushPendingProgress(async (p) => { posted.push(p) })
    expect(posted).toHaveLength(1)
  })
})
