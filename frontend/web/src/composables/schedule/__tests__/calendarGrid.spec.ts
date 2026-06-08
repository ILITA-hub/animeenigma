// frontend/web/src/composables/schedule/__tests__/calendarGrid.spec.ts
import { describe, it, expect } from 'vitest'
import { startOfDay, isSameDay, weekStart, weekDays, monthGridDays, monthGridRange } from '../calendarGrid'

const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

describe('startOfDay / isSameDay', () => {
  it('zeros the time', () => {
    const s = startOfDay(new Date(2026, 5, 8, 17, 30))
    expect(s.getHours()).toBe(0)
    expect(s.getMinutes()).toBe(0)
  })
  it('isSameDay ignores time', () => {
    expect(isSameDay(new Date(2026, 5, 8, 1), new Date(2026, 5, 8, 23))).toBe(true)
    expect(isSameDay(new Date(2026, 5, 8), new Date(2026, 5, 9))).toBe(false)
  })
})

describe('weekStart (Monday-first)', () => {
  it('Monday returns itself', () => {
    expect(iso(weekStart(new Date(2026, 5, 8)))).toBe('2026-06-08')
  })
  it('Sunday returns the preceding Monday', () => {
    expect(iso(weekStart(new Date(2026, 5, 14)))).toBe('2026-06-08')
  })
})

describe('weekDays', () => {
  it('returns Mon..Sun (7 days)', () => {
    const days = weekDays(new Date(2026, 5, 10))
    expect(days).toHaveLength(7)
    expect(iso(days[0])).toBe('2026-06-08')
    expect(iso(days[6])).toBe('2026-06-14')
  })
})

describe('monthGridDays', () => {
  it('June 2026 starts on Monday -> first cell is June 1', () => {
    const days = monthGridDays(new Date(2026, 5, 8))
    expect(iso(days[0])).toBe('2026-06-01')
    expect(days.length % 7).toBe(0)
  })
  it('July 2026 starts Wednesday -> grid begins on June 29 (Mon)', () => {
    const days = monthGridDays(new Date(2026, 6, 1))
    expect(iso(days[0])).toBe('2026-06-29')
    expect(days.length % 7).toBe(0)
  })
})

describe('monthGridRange', () => {
  it('start is inclusive first cell, end is exclusive day after last cell', () => {
    const { start, end } = monthGridRange(new Date(2026, 6, 1))
    expect(iso(start)).toBe('2026-06-29')
    expect(iso(end)).toBe('2026-08-03')
  })
})
