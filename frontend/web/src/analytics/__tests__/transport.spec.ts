import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { AnalyticsEvent } from '../types'

function evt(t: AnalyticsEvent['event_type']): AnalyticsEvent {
  return { event_type: t, timestamp: new Date().toISOString() }
}

describe('transport', () => {
  let beacon: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    vi.resetModules()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom has no sendBeacon by default
    navigator.sendBeacon = beacon
    localStorage.clear()
  })

  it('flush sends a single envelope with buffered events', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/api/analytics/collect', maxBatch: 100, flushMs: 999999 })
    t.enqueue(evt('pageview'))
    t.enqueue(evt('click'))
    t.flush('manual')
    expect(beacon).toHaveBeenCalledTimes(1)
    const [url, blob] = beacon.mock.calls[0]
    expect(url).toContain('/api/analytics/collect')
    const text = await (blob as Blob).text()
    const env = JSON.parse(text)
    expect(env.events).toHaveLength(2)
    expect(env.anonymous_id).toBeTruthy()
    expect(env.session_id).toBeTruthy()
  })

  it('flush is a no-op when the buffer is empty', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 100, flushMs: 999999 })
    t.flush('manual')
    expect(beacon).not.toHaveBeenCalled()
  })

  it('auto-flushes when the buffer reaches maxBatch', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 2, flushMs: 999999 })
    t.enqueue(evt('click'))
    t.enqueue(evt('click')) // hits maxBatch -> flush
    expect(beacon).toHaveBeenCalledTimes(1)
  })

  it('falls back to fetch keepalive when sendBeacon returns false', async () => {
    const { Transport } = await import('../transport')
    beacon.mockReturnValue(false)
    const fetchMock = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('fetch', fetchMock)
    const t = new Transport({ endpoint: '/x', maxBatch: 100, flushMs: 999999 })
    t.enqueue(evt('pageview'))
    t.flush('manual')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const opts = fetchMock.mock.calls[0][1]
    expect(opts.keepalive).toBe(true)
  })
})
