import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  noteMaskedAnalyticsPath,
  analyticsEndpoint,
  maskedOverrideFor,
  maskedEndpointFor,
  markBlockedFromError,
  isMaskedAnalyticsUrl,
  probeAnalyticsReachability,
  shipAnalyticsPayload,
  __resetAnalyticsTransportForTest,
} from '../analyticsTransport'

const MASKED = '/api/0123456789abcdef01234567'

describe('analyticsTransport', () => {
  beforeEach(() => {
    __resetAnalyticsTransportForTest()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(null, { status: 200 }))))
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('defaults to the primary endpoints', () => {
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
    expect(analyticsEndpoint('player-events')).toBe('/api/analytics/player-events')
    expect(maskedOverrideFor('collect')).toBeNull()
  })

  it('accepts only well-formed masked bases', () => {
    noteMaskedAnalyticsPath('https://evil.example/steal')
    noteMaskedAnalyticsPath('/api/short')
    expect(isMaskedAnalyticsUrl(MASKED + '/c')).toBe(false)
    noteMaskedAnalyticsPath(MASKED)
    expect(isMaskedAnalyticsUrl(MASKED + '/c')).toBe(true)
  })

  it('markBlockedFromError flips only on TypeError with a known masked base', () => {
    noteMaskedAnalyticsPath(MASKED)
    expect(markBlockedFromError(new Error('boom'))).toBe(false)
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
    expect(markBlockedFromError(new TypeError('Failed to fetch'))).toBe(true)
    expect(analyticsEndpoint('collect')).toBe(`${MASKED}/c`)
    expect(analyticsEndpoint('client-errors')).toBe(`${MASKED}/e`)
    expect(analyticsEndpoint('player-events')).toBe(`${MASKED}/p`)
  })

  it('does not flip without a masked base (fail-open)', () => {
    expect(markBlockedFromError(new TypeError('Failed to fetch'))).toBe(false)
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
  })

  it('probe fires once, only after a masked base is known, and flips on TypeError', async () => {
    probeAnalyticsReachability() // no masked base yet → no-op
    expect(fetch).not.toHaveBeenCalled()

    noteMaskedAnalyticsPath(MASKED)
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))))
    probeAnalyticsReachability()
    probeAnalyticsReachability() // second call must be a no-op
    expect(fetch).toHaveBeenCalledTimes(1)
    await Promise.resolve()
    await Promise.resolve()
    expect(analyticsEndpoint('collect')).toBe(`${MASKED}/c`)
  })

  it('maskedEndpointFor resolves whenever the base is known, independent of blocked', () => {
    expect(maskedEndpointFor('collect')).toBeNull()
    noteMaskedAnalyticsPath(MASKED)
    expect(maskedEndpointFor('collect')).toBe(`${MASKED}/c`)
    expect(maskedOverrideFor('collect')).toBeNull() // not blocked yet
  })

  describe('shipAnalyticsPayload', () => {
    it('ships via fetch keepalive to the primary endpoint (never sendBeacon)', () => {
      const beacon = vi.fn().mockReturnValue(true)
      Object.defineProperty(navigator, 'sendBeacon', {
        configurable: true,
        writable: true,
        value: beacon,
      })
      shipAnalyticsPayload('collect', '{"events":[]}')
      expect(beacon).not.toHaveBeenCalled()
      expect(fetch).toHaveBeenCalledTimes(1)
      const [url, init] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
      expect(url).toBe('/api/analytics/collect')
      expect(init.keepalive).toBe(true)
      expect(init.method).toBe('POST')
      expect(init.credentials).toBe('include')
    })

    it('flips to the masked alias on TypeError and retries the batch once', async () => {
      noteMaskedAnalyticsPath(MASKED)
      const calls: string[] = []
      vi.stubGlobal(
        'fetch',
        vi.fn((url: string) => {
          calls.push(url)
          return calls.length === 1
            ? Promise.reject(new TypeError('Failed to fetch'))
            : Promise.resolve(new Response(null, { status: 204 }))
        }),
      )
      shipAnalyticsPayload('player-events', '{"events":[]}')
      await vi.waitFor(() => expect(calls).toHaveLength(2))
      expect(calls[0]).toBe('/api/analytics/player-events')
      expect(calls[1]).toBe(`${MASKED}/p`)
      expect(analyticsEndpoint('collect')).toBe(`${MASKED}/c`) // session stays flipped
    })

    it('goes straight to the masked alias once the session is blocked', () => {
      noteMaskedAnalyticsPath(MASKED)
      markBlockedFromError(new TypeError('Failed to fetch'))
      shipAnalyticsPayload('client-errors', '{"errors":[]}')
      expect((fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe(`${MASKED}/e`)
    })

    it('never throws when fetch is unavailable', () => {
      vi.stubGlobal('fetch', undefined)
      expect(() => shipAnalyticsPayload('collect', '{}')).not.toThrow()
    })
  })
})
