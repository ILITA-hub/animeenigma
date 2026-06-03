import { describe, it, expect } from 'vitest'
import { newTraceparent } from '../traceparent'

describe('newTraceparent', () => {
  it('produces a valid W3C traceparent: 00-<32hex>-<16hex>-01', () => {
    const { header, traceId } = newTraceparent()
    expect(header).toMatch(/^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/)
    expect(traceId).toMatch(/^[0-9a-f]{32}$/)
    expect(header).toContain(traceId)
  })

  it('does not emit an all-zero trace id', () => {
    for (let i = 0; i < 20; i++) {
      expect(newTraceparent().traceId).not.toBe('0'.repeat(32))
    }
  })

  it('is unique across calls', () => {
    const a = newTraceparent().traceId
    const b = newTraceparent().traceId
    expect(a).not.toBe(b)
  })
})
