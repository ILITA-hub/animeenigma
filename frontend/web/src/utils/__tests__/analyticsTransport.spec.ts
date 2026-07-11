import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  noteMaskedAnalyticsPath,
  analyticsEndpoint,
  maskedOverrideFor,
  markBlockedFromError,
  isMaskedAnalyticsUrl,
  probeAnalyticsReachability,
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
})
