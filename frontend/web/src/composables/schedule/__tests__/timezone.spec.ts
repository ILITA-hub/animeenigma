import { describe, it, expect } from 'vitest'
import { wallClockDate, tzOffsetMinutes, formatUtcOffset } from '../timezone'

// June dates → DST active in Europe/Berlin and America/New_York.
const instant = new Date('2026-06-13T12:00:00Z')

describe('wallClockDate', () => {
  it('re-expresses the instant as Tokyo wall clock (UTC+9)', () => {
    const d = wallClockDate(instant, 'Asia/Tokyo')
    expect(d.getHours()).toBe(21)
    expect(d.getMinutes()).toBe(0)
    expect(d.getDate()).toBe(13)
  })

  it('UTC wall clock matches the Z fields', () => {
    const d = wallClockDate(instant, 'UTC')
    expect(d.getHours()).toBe(12)
    expect(d.getDate()).toBe(13)
  })

  it('shifts the calendar DAY when the zone crosses midnight', () => {
    const late = new Date('2026-06-13T23:00:00Z')
    const d = wallClockDate(late, 'Asia/Tokyo')
    expect(d.getDate()).toBe(14)
    expect(d.getHours()).toBe(8)
  })

  it('UTC+3 example from the feature request: 12:00 UTC → 15:00', () => {
    const d = wallClockDate(instant, 'Europe/Moscow')
    expect(d.getHours()).toBe(15)
  })

  it('returns the date unchanged without a tz', () => {
    expect(wallClockDate(instant)).toBe(instant)
  })

  it('degrades to browser-local on an invalid tz instead of throwing', () => {
    const d = wallClockDate(instant, 'Not/AZone')
    expect(d.getTime()).toBe(instant.getTime())
  })
})

describe('tzOffsetMinutes / formatUtcOffset', () => {
  it('whole-hour offsets', () => {
    expect(tzOffsetMinutes('Europe/Moscow', instant)).toBe(180)
    expect(tzOffsetMinutes('Asia/Tokyo', instant)).toBe(540)
    expect(formatUtcOffset('Europe/Moscow', instant)).toBe('UTC+3')
    expect(formatUtcOffset('UTC', instant)).toBe('UTC+0')
  })

  it('negative offset (New York, June = EDT)', () => {
    expect(tzOffsetMinutes('America/New_York', instant)).toBe(-240)
    expect(formatUtcOffset('America/New_York', instant)).toBe('UTC-4')
  })

  it('fractional offset (Kolkata)', () => {
    expect(formatUtcOffset('Asia/Kolkata', instant)).toBe('UTC+5:30')
  })
})
