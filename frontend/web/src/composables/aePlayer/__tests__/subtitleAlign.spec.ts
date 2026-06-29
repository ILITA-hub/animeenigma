import { describe, it, expect } from 'vitest'
import { cuesToIntervals, overlapDuration, bestOffset, classifyFrame, round1 } from '../subtitleAlign'

describe('round1', () => {
  it('rounds to one decimal without float fuzz', () => {
    expect(round1(0.1 + 0.2)).toBe(0.3)
    expect(round1(2.449)).toBe(2.4)
  })
})

describe('cuesToIntervals', () => {
  it('sorts and merges overlapping/adjacent cues', () => {
    expect(cuesToIntervals([{ start: 5, end: 6 }, { start: 1, end: 2 }, { start: 2, end: 3 }]))
      .toEqual([{ start: 1, end: 3 }, { start: 5, end: 6 }])
  })
  it('returns [] for empty input', () => { expect(cuesToIntervals([])).toEqual([]) })
})

describe('overlapDuration', () => {
  it('measures intersection with b shifted by +delta', () => {
    const a = [{ start: 10, end: 12 }], b = [{ start: 8, end: 10 }]
    expect(overlapDuration(a, b, 0)).toBeCloseTo(0, 5)
    expect(overlapDuration(a, b, 2)).toBeCloseTo(2, 5)
  })
})

describe('bestOffset', () => {
  it('recovers a constant offset: subs 2s EARLY -> +2 (positive = later)', () => {
    const r = bestOffset([{ start: 10, end: 12 }, { start: 20, end: 23 }], [{ start: 8, end: 10 }, { start: 18, end: 21 }])
    expect(r.offset).toBeCloseTo(2, 1)
    expect(r.confidence).toBeGreaterThan(0.15)
  })
  it('recovers a negative offset: subs 1.5s LATE -> -1.5', () => {
    expect(bestOffset([{ start: 10, end: 12 }, { start: 30, end: 33 }], [{ start: 11.5, end: 13.5 }, { start: 31.5, end: 34.5 }]).offset)
      .toBeCloseTo(-1.5, 1)
  })
  it('reports low confidence with no clear peak', () => {
    expect(bestOffset([{ start: 0, end: 60 }], [{ start: 0, end: 60 }]).confidence).toBeLessThan(0.15)
  })
  it('returns offset 0 / confidence 0 for empty inputs', () => {
    expect(bestOffset([], [{ start: 1, end: 2 }])).toEqual({ offset: 0, confidence: 0 })
    expect(bestOffset([{ start: 1, end: 2 }], [])).toEqual({ offset: 0, confidence: 0 })
  })
})

describe('classifyFrame', () => {
  const fftSize = 2048, sampleRate = 48000, bins = fftSize / 2, binHz = sampleRate / fftSize
  const lo = Math.floor(300 / binHz), hi = Math.ceil(3400 / binHz)
  it('true when loud energy sits in the 300-3400Hz band', () => {
    const freq = new Uint8Array(bins)
    for (let i = lo; i <= hi; i++) freq[i] = 200
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(true)
  })
  it('false when loud energy sits OUTSIDE the band (ratio gate)', () => {
    const freq = new Uint8Array(bins)
    for (let i = hi + 1; i < bins; i++) freq[i] = 200   // high mean, but out of band
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(false)
  })
  it('false on near-silence even if band-weighted (energy gate)', () => {
    const freq = new Uint8Array(bins)
    for (let i = lo; i <= hi; i++) freq[i] = 3          // ratio high, mean tiny
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(false)
  })
})
