import { describe, it, expect } from 'vitest'
import { subtitleBgColor } from './subtitleStyle'

describe('subtitleBgColor', () => {
  it('maps 0 % to a fully transparent black', () => {
    expect(subtitleBgColor(0)).toBe('rgba(0, 0, 0, 0.00)')
  })

  it('caps 100 % at the 0.85 alpha ceiling (never fully occludes the video)', () => {
    expect(subtitleBgColor(100)).toBe('rgba(0, 0, 0, 0.85)')
  })

  it('scales linearly in between and rounds to 2 dp', () => {
    // 45 → 0.45 * 0.85 = 0.3825 → 0.38
    expect(subtitleBgColor(45)).toBe('rgba(0, 0, 0, 0.38)')
  })
})
