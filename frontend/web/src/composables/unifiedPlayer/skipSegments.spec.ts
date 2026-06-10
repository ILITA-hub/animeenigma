import { describe, it, expect } from 'vitest'
import { segmentsToChapters, activeSkipSegment } from './skipSegments'

describe('segmentsToChapters', () => {
  it('maps op+ed segments to pct-based chapters', () => {
    expect(
      segmentsToChapters({ start: 60, end: 150 }, { start: 1300, end: 1390 }, 1400),
    ).toEqual([
      { kind: 'intro', startPct: (60 / 1400) * 100, widthPct: (90 / 1400) * 100 },
      { kind: 'outro', startPct: (1300 / 1400) * 100, widthPct: (90 / 1400) * 100 },
    ])
  })

  it('returns [] until the duration is known (no fake markers — F6)', () => {
    expect(segmentsToChapters({ start: 60, end: 150 }, null, 0)).toEqual([])
  })

  it('clamps segments to the duration and drops degenerate (<1s clamped) ones', () => {
    // After clamping to 1400s this segment is 0.5s wide → dropped
    expect(segmentsToChapters({ start: 1399.5, end: 1500 }, null, 1400)).toEqual([])
    const [c] = segmentsToChapters({ start: 1300, end: 1500 }, null, 1400)
    expect(c.startPct + c.widthPct).toBeCloseTo(100)
  })

  it('handles null segments', () => {
    expect(segmentsToChapters(null, null, 1400)).toEqual([])
  })
})

describe('activeSkipSegment', () => {
  const op = { start: 60, end: 150 }
  const ed = { start: 1300, end: 1390 }

  it('offers the intro inside the OP window', () => {
    expect(activeSkipSegment(100, op, ed)).toEqual({ kind: 'intro', end: 150 })
  })

  it('offers the outro inside the ED window', () => {
    expect(activeSkipSegment(1320, op, ed)).toEqual({ kind: 'outro', end: 1390 })
  })

  it('offers nothing outside both windows', () => {
    expect(activeSkipSegment(500, op, ed)).toBeNull()
  })

  it('stops offering 1s before the segment end (no ~no-op seeks)', () => {
    expect(activeSkipSegment(149.5, op, ed)).toBeNull()
  })

  it('handles null segments', () => {
    expect(activeSkipSegment(100, null, null)).toBeNull()
  })
})
