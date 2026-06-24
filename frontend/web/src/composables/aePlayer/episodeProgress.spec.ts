import { describe, it, expect } from 'vitest'
import { progressRowsToMap, fmtResume } from './episodeProgress'

describe('progressRowsToMap', () => {
  it('keys rows by episode number with clamped pct', () => {
    expect(
      progressRowsToMap([
        { episode_number: 1, progress: 300, duration: 600, completed: false },
        { episode_number: 2, progress: 600, duration: 600, completed: true },
      ]),
    ).toEqual({
      1: { pct: 0.5, sec: 300, completed: false },
      2: { pct: 1, sec: 600, completed: true },
    })
  })

  it('clamps pct to 1 when progress exceeds duration', () => {
    expect(progressRowsToMap([{ episode_number: 3, progress: 900, duration: 600 }])[3].pct).toBe(1)
  })

  it('uses pct 0 when duration is unknown', () => {
    expect(progressRowsToMap([{ episode_number: 4, progress: 120 }])).toEqual({
      4: { pct: 0, sec: 120, completed: false },
    })
  })

  it('skips rows without an episode number', () => {
    expect(progressRowsToMap([{ progress: 100, duration: 200 }])).toEqual({})
  })

  it('treats missing progress as 0', () => {
    expect(progressRowsToMap([{ episode_number: 5, duration: 600 }])).toEqual({
      5: { pct: 0, sec: 0, completed: false },
    })
  })

  it('returns an empty map for an empty list', () => {
    expect(progressRowsToMap([])).toEqual({})
  })
})

describe('fmtResume', () => {
  it('formats seconds as m:ss', () => {
    expect(fmtResume(0)).toBe('0:00')
    expect(fmtResume(5)).toBe('0:05')
    expect(fmtResume(65)).toBe('1:05')
    expect(fmtResume(600)).toBe('10:00')
  })

  it('floors fractional seconds', () => {
    expect(fmtResume(90.9)).toBe('1:30')
  })
})
