import { describe, it, expect } from 'vitest'
import { computeAutoMarkThreshold, AUTO_MARK_FALLBACK } from './kodikAutoMark'

describe('computeAutoMarkThreshold', () => {
  it('prefers the live Kodik duration when known', () => {
    // Needy Girl Overdose case: catalog says 24 min (Shikimori nominal), the
    // real streamed episode runs 1115s (~18.6 min) — the Kodik signal must
    // win so a full watch actually crosses the threshold.
    expect(computeAutoMarkThreshold(24, 1115)).toBe(Math.round(0.9 * 1115))
    expect(computeAutoMarkThreshold(24, 1115)).toBeLessThan(1115)
  })

  it('caps the Kodik-derived threshold at the 20-minute fallback', () => {
    expect(computeAutoMarkThreshold(0, 1600)).toBe(AUTO_MARK_FALLBACK)
  })

  it('falls back to the catalog duration when Kodik has not reported yet', () => {
    expect(computeAutoMarkThreshold(24, 0)).toBe(AUTO_MARK_FALLBACK) // min(1200, 1296)
    expect(computeAutoMarkThreshold(10, 0)).toBe(540) // 0.9 * 10 * 60, well under the cap
  })

  it('falls back to the legacy 20-minute rule when nothing is known', () => {
    expect(computeAutoMarkThreshold(0, 0)).toBe(AUTO_MARK_FALLBACK)
  })
})
