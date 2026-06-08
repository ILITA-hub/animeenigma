// AR-FE-01/AR-FE-03 regression: assert that activity-register fields (source,
// trace_id, operation, target, …) land at the TOP LEVEL of the SERIALIZED
// analytics event, not buried under `properties`. This covers the exact
// serialization boundary the prior mocks skipped: rum.spec.ts asserted on the
// `emit` props bag, but the bug was that track()/page() nested ALL props under
// `properties`, so the collector's top-level wireEvent never read them and the
// FE→BE trace_id join returned 0 rows.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

async function lastEnvelope(beacon: ReturnType<typeof vi.fn>) {
  const blob = beacon.mock.calls.at(-1)![1] as Blob
  return JSON.parse(await blob.text())
}

describe('register-field serialization boundary', () => {
  let beacon: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom lacks sendBeacon
    navigator.sendBeacon = beacon
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('track() lifts source + trace_id to the TOP LEVEL of the serialized event', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.track('fe.call', {
      source: 'fe',
      trace_id: 'abc123def456',
      operation: 'GET anime-detail',
      target: '/api/anime/42',
      target_kind: 'route',
    })
    analytics.flushNow()
    const env = await lastEnvelope(beacon)
    const ev = env.events.find((e: { event_name?: string }) => e.event_name === 'fe.call')
    expect(ev).toBeTruthy()
    // Top-level register fields — the collector reads these directly.
    expect(ev.source).toBe('fe')
    expect(ev.trace_id).toBe('abc123def456')
    expect(ev.operation).toBe('GET anime-detail')
    expect(ev.target).toBe('/api/anime/42')
    expect(ev.target_kind).toBe('route')
  })

  it('track() does NOT bury register fields inside properties', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.track('fe.call', { source: 'fe', trace_id: 't1' })
    analytics.flushNow()
    const env = await lastEnvelope(beacon)
    const ev = env.events.find((e: { event_name?: string }) => e.event_name === 'fe.call')
    // properties is omitted (or at least carries no register keys).
    const props = ev.properties ?? {}
    expect(props.source).toBeUndefined()
    expect(props.trace_id).toBeUndefined()
  })

  it('rum.resource emit serializes source==="fe_rum" + target_kind top-level', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    // Mirror exactly what rum.ts emits via analytics.track().
    analytics.track('rum.resource', {
      source: 'fe_rum',
      target_kind: 'host',
      target: 'cdn.example.com',
      requests: 3,
      duration_ms: 120,
    })
    analytics.flushNow()
    const env = await lastEnvelope(beacon)
    const ev = env.events.find((e: { event_name?: string }) => e.event_name === 'rum.resource')
    expect(ev.source).toBe('fe_rum')
    expect(ev.target_kind).toBe('host')
    expect(ev.target).toBe('cdn.example.com')
    expect(ev.requests).toBe(3)
    expect(ev.duration_ms).toBe(120)
  })

  it('keeps arbitrary (non-register) props under properties', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.track('custom.evt', { source: 'fe', foo: 'bar', count: 7 })
    analytics.flushNow()
    const env = await lastEnvelope(beacon)
    const ev = env.events.find((e: { event_name?: string }) => e.event_name === 'custom.evt')
    expect(ev.source).toBe('fe') // register field lifted
    expect(ev.properties).toEqual({ foo: 'bar', count: 7 }) // arbitrary props stay nested
  })

  it('omits properties entirely when only register fields were supplied', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.track('fe.call', { source: 'fe', trace_id: 't2' })
    analytics.flushNow()
    const env = await lastEnvelope(beacon)
    const ev = env.events.find((e: { event_name?: string }) => e.event_name === 'fe.call')
    expect(ev.properties).toBeUndefined()
  })
})
