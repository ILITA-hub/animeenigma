import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  reportFeError,
  flushFeErrors,
  __resetFeErrorLogForTest,
  __getFeErrorBufferForTest,
  __runSuppressedPassForTest,
} from '../feErrorLog'

describe('feErrorLog', () => {
  beforeEach(() => {
    __resetFeErrorLogForTest()
    // Default: sendBeacon "succeeds" so the buffer is sent without fetch.
    Object.defineProperty(navigator, 'sendBeacon', {
      configurable: true,
      writable: true,
      value: vi.fn(() => true),
    })
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve()))
  })
  afterEach(() => {
    __resetFeErrorLogForTest()
    vi.unstubAllGlobals()
  })

  it('buffers a reported error and flushes to the client-errors endpoint', () => {
    // Force the fetch fallback so we can read the JSON body.
    const beacon = navigator.sendBeacon as ReturnType<typeof vi.fn>
    beacon.mockReturnValue(false)
    reportFeError({ kind: 'http', message: 'Request failed', url: '/api/anime/x/ae/stream', status: 404, provider: 'ae' })
    expect(__getFeErrorBufferForTest()).toHaveLength(1)

    flushFeErrors('test')
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, opts] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/analytics/client-errors')
    const body = JSON.parse((opts as RequestInit).body as string)
    expect(body.errors).toHaveLength(1)
    expect(body.errors[0]).toMatchObject({ kind: 'http', status: 404, provider: 'ae' })
    expect(body.ctx).toHaveProperty('user_agent')
  })

  it('dedups repeats and emits one suppressed summary with the count', () => {
    for (let i = 0; i < 5; i++) {
      reportFeError({ kind: 'player', message: 'Stream unavailable', provider: 'ae' })
    }
    // Only the first occurrence is buffered immediately.
    expect(__getFeErrorBufferForTest()).toHaveLength(1)

    __runSuppressedPassForTest()
    const buf = __getFeErrorBufferForTest()
    const summary = buf.find((e) => e.kind === 'suppressed')
    expect(summary).toBeTruthy()
    expect(summary?.count).toBe(4) // 5 total − 1 already shipped
    expect(summary?.message).toBe('Stream unavailable')
  })

  it('rate-caps distinct errors and emits a single cap marker', () => {
    for (let i = 0; i < 25; i++) {
      reportFeError({ kind: 'js', message: `boom ${i}` })
    }
    // 20/min accepted (a size-flush fired at 20), then a cap marker; rest dropped.
    expect((navigator.sendBeacon as ReturnType<typeof vi.fn>)).toHaveBeenCalledTimes(1)
    const buf = __getFeErrorBufferForTest()
    expect(buf).toHaveLength(1)
    expect(buf[0].kind).toBe('cap')
  })

  it('never reports failures of its own beacon/collector endpoints (loop guard)', () => {
    reportFeError({ kind: 'http', message: 'failed', url: '/api/analytics/client-errors', status: 500 })
    reportFeError({ kind: 'http', message: 'failed', url: '/api/analytics/collect', status: 500 })
    expect(__getFeErrorBufferForTest()).toHaveLength(0)
  })

  it('flush is a no-op when the buffer is empty', () => {
    flushFeErrors('test')
    expect((navigator.sendBeacon as ReturnType<typeof vi.fn>)).not.toHaveBeenCalled()
  })
})
