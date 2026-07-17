import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { AnalyticsEvent } from '../types'

function evt(t: AnalyticsEvent['event_type']): AnalyticsEvent {
  return { event_type: t, timestamp: new Date().toISOString() }
}

// Transport ships via fetch keepalive only (shipAnalyticsPayload, AUTO-629);
// sendBeacon must never be touched — $ping blockers eat beacons silently.
describe('transport', () => {
  let beacon: ReturnType<typeof vi.fn>
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    vi.resetModules()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom has no sendBeacon by default
    navigator.sendBeacon = beacon
    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    localStorage.clear()
  })

  it('flush sends a single envelope with buffered events via fetch keepalive', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/api/analytics/collect', maxBatch: 100, flushMs: 999999 })
    t.enqueue(evt('pageview'))
    t.enqueue(evt('click'))
    t.flush('manual')
    expect(beacon).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toContain('/api/analytics/collect')
    expect((init as RequestInit).keepalive).toBe(true)
    const env = JSON.parse((init as RequestInit).body as string)
    expect(env.events).toHaveLength(2)
    expect(env.anonymous_id).toBeTruthy()
    expect(env.session_id).toBeTruthy()
  })

  it('flush is a no-op when the buffer is empty', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 100, flushMs: 999999 })
    t.flush('manual')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('auto-flushes when the buffer reaches maxBatch', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 2, flushMs: 999999 })
    t.enqueue(evt('click'))
    t.enqueue(evt('click')) // hits maxBatch -> flush
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('splits oversized batches and ships each chunk', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 1000, flushMs: 999999 })
    const big = 'x'.repeat(40 * 1024)
    t.enqueue({ ...evt('pageview'), title: big })
    t.enqueue({ ...evt('pageview'), title: big })
    t.flush('manual') // ~80KB envelope > 60KB cap → split into 2 sends
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
