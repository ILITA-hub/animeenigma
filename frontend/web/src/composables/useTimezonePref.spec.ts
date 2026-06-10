import { describe, it, expect, beforeEach } from 'vitest'
import { useTimezonePref, TIMEZONE_CHOICES } from './useTimezonePref'

describe('useTimezonePref', () => {
  beforeEach(() => {
    localStorage.removeItem('pref:timezone')
    useTimezonePref().setPref('auto') // module singleton — reset between tests
  })

  it('defaults to auto = browser timezone', () => {
    const { pref, timezone, browserTimezone } = useTimezonePref()
    expect(pref.value).toBe('auto')
    expect(timezone.value).toBe(browserTimezone)
  })

  it('persists an explicit zone and resolves it', () => {
    const { setPref, timezone, pref } = useTimezonePref()
    setPref('Asia/Tokyo')
    expect(pref.value).toBe('Asia/Tokyo')
    expect(timezone.value).toBe('Asia/Tokyo')
    expect(localStorage.getItem('pref:timezone')).toBe('Asia/Tokyo')
  })

  it('is a shared singleton across call sites', () => {
    const a = useTimezonePref()
    const b = useTimezonePref()
    a.setPref('Europe/Moscow')
    expect(b.timezone.value).toBe('Europe/Moscow')
  })

  it('rejects garbage values back to auto', () => {
    const { setPref, pref } = useTimezonePref()
    setPref('Not/AZone')
    expect(pref.value).toBe('auto')
  })

  it('curated list has unique valid IANA values', () => {
    const values = TIMEZONE_CHOICES.map((c) => c.value)
    expect(new Set(values).size).toBe(values.length)
    for (const v of values) {
      expect(() => new Intl.DateTimeFormat('en-US', { timeZone: v })).not.toThrow()
    }
  })
})
