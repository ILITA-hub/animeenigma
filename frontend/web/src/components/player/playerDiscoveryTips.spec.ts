import { describe, expect, it } from 'vitest'
import { pickDiscoveryTipIndex } from './playerDiscoveryTips'

describe('pickDiscoveryTipIndex', () => {
  it('maps the random sample across every available tip', () => {
    expect(pickDiscoveryTipIndex(4, -1, () => 0)).toBe(0)
    expect(pickDiscoveryTipIndex(4, -1, () => 0.26)).toBe(1)
    expect(pickDiscoveryTipIndex(4, -1, () => 0.99)).toBe(3)
  })

  it('never repeats the currently visible tip', () => {
    expect(pickDiscoveryTipIndex(4, 1, () => 0)).toBe(0)
    expect(pickDiscoveryTipIndex(4, 1, () => 0.34)).toBe(2)
    expect(pickDiscoveryTipIndex(4, 1, () => 0.99)).toBe(3)
  })

  it('handles empty, singleton, and out-of-range random inputs safely', () => {
    expect(pickDiscoveryTipIndex(0, -1, () => 0.5)).toBe(-1)
    expect(pickDiscoveryTipIndex(1, 0, () => 0.5)).toBe(0)
    expect(pickDiscoveryTipIndex(3, -1, () => -2)).toBe(0)
    expect(pickDiscoveryTipIndex(3, -1, () => 2)).toBe(2)
    expect(pickDiscoveryTipIndex(3, -1, () => Number.NaN)).toBe(0)
  })
})
