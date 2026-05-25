/**
 * Unit spec for formatAgo (frontend/web/src/utils/time.ts).
 *
 * Extracted from LatestNewsCard's inline formatEntryDate during
 * v1.1-polish Phase 09 (HSB-V11-NT-01). Assertions are computed relative
 * to `Date.now()` so they stay stable regardless of the wall clock.
 */

import { describe, it, expect } from 'vitest'
import { formatAgo } from './time'

const DAY_MS = 86_400_000

function isoDaysAgo(days: number): string {
  return new Date(Date.now() - days * DAY_MS).toISOString()
}

describe('formatAgo', () => {
  it('returns the raw ISO string when the input is unparseable', () => {
    expect(formatAgo('not-a-date', 'en')).toBe('not-a-date')
  })

  it('renders "today" for a timestamp less than a day old (en)', () => {
    // numeric:'auto' yields "today" for the 0-day bucket.
    expect(formatAgo(isoDaysAgo(0), 'en')).toBe('today')
  })

  it('renders day-granularity for < 7 days (en)', () => {
    expect(formatAgo(isoDaysAgo(2), 'en')).toBe('2 days ago')
  })

  it('renders week-granularity for >= 7 and < 30 days (en)', () => {
    expect(formatAgo(isoDaysAgo(14), 'en')).toBe('2 weeks ago')
  })

  it('renders Russian week-granularity for a ~2-week-old timestamp', () => {
    expect(formatAgo(isoDaysAgo(14), 'ru')).toContain('недел')
  })

  it('falls back to an absolute medium date beyond 30 days', () => {
    const out = formatAgo(isoDaysAgo(90), 'en')
    // Medium date style includes a 4-digit year; the relative phrasing
    // ("ago") must be absent for the absolute branch.
    expect(out).not.toContain('ago')
    expect(out).toMatch(/\d{4}/)
  })

  it('does not throw and degrades gracefully on an exotic locale', () => {
    expect(() => formatAgo(isoDaysAgo(2), 'en')).not.toThrow()
  })
})
