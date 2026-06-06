import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// A fake PerformanceResourceTiming-shaped entry: only the fields rum.ts reads.
function entry(name: string, duration: number): PerformanceResourceTiming {
  return { name, duration } as unknown as PerformanceResourceTiming
}

// Capture the observer callback that initRum registers so the test can invoke it
// synchronously with a fake entry list (no real network / no real timing).
function stubObserver(): { fire: (entries: PerformanceResourceTiming[]) => void } {
  let cb: PerformanceObserverCallback | null = null
  const handle = { fire: (_entries: PerformanceResourceTiming[]) => {} }
  class FakePerformanceObserver {
    constructor(callback: PerformanceObserverCallback) {
      cb = callback
    }
    observe(): void {}
    disconnect(): void {}
    takeRecords(): PerformanceEntryList {
      return []
    }
  }
  vi.stubGlobal('PerformanceObserver', FakePerformanceObserver)
  handle.fire = (entries: PerformanceResourceTiming[]) => {
    const list = { getEntries: () => entries } as unknown as PerformanceObserverEntryList
    cb?.(list, {} as PerformanceObserver)
  }
  return handle
}

describe('rum', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('drops self-host resource entries (no emit for location.host)', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([entry(`https://${location.host}/app.js`, 12)])
    expect(emit).not.toHaveBeenCalled()
  })

  it('aggregates two same-3rd-party-host entries into ONE row with requests===2', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([
      entry('https://cdn.example.com/a/seg1.ts?token=abc', 10),
      entry('https://cdn.example.com/b/seg2.ts?token=def', 25),
    ])
    expect(emit).toHaveBeenCalledTimes(1)
    const props = emit.mock.calls[0][1] as Record<string, unknown>
    expect(props.requests).toBe(2)
  })

  it('duration_ms equals the rounded sum of the two durations', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([
      entry('https://cdn.example.com/seg1.ts', 10.4),
      entry('https://cdn.example.com/seg2.ts', 25.3),
    ])
    const props = emit.mock.calls[0][1] as Record<string, unknown>
    expect(props.duration_ms).toBe(36) // round(35.7)
  })

  it('target is the host ONLY — no path, no query, no signed token', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([entry('https://cdn.example.com/sub/seg.ts?tham=h&token=secret', 5)])
    const props = emit.mock.calls[0][1] as Record<string, unknown>
    expect(props.target).toBe('cdn.example.com')
    expect(String(props.target)).not.toContain('/')
    expect(String(props.target)).not.toContain('?')
    expect(String(props.target)).not.toContain('token')
  })

  it('emitted props carry source==="fe_rum" and NO byte fields', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([entry('https://cdn.example.com/seg.ts', 5)])
    const [name, props] = emit.mock.calls[0] as [string, Record<string, unknown>]
    expect(name).toBe('rum.resource')
    expect(props.source).toBe('fe_rum')
    expect(props.target_kind).toBe('host')
    expect(Object.prototype.hasOwnProperty.call(props, 'bytes_in')).toBe(false)
    expect(Object.prototype.hasOwnProperty.call(props, 'bytes_out')).toBe(false)
  })

  it('skips entries whose name is not a parseable URL', async () => {
    const observer = stubObserver()
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    initRum(emit)
    observer.fire([entry('not a url', 5)])
    expect(emit).not.toHaveBeenCalled()
  })

  it('is a silent no-op when PerformanceObserver is absent', async () => {
    vi.stubGlobal('PerformanceObserver', undefined)
    const emit = vi.fn()
    const { initRum } = await import('../rum')
    expect(() => initRum(emit)).not.toThrow()
    expect(emit).not.toHaveBeenCalled()
  })
})
