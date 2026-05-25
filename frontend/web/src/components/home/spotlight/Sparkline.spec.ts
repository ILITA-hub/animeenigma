/**
 * Workstream hero-spotlight — v1.1-polish Phase 08 (platform-stats-refactor).
 *
 * Vitest spec for Sparkline.vue. Verifies the pure-SVG polyline geometry,
 * the data-points mirror, and the defensive single-/flat-series handling.
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'

import Sparkline from './Sparkline.vue'

describe('Sparkline', () => {
  it('renders a single <svg> with a <polyline> and no chart-library deps', () => {
    const wrapper = mount(Sparkline, { props: { data: [1, 2, 3, 4, 5, 6, 7] } })
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.find('polyline').exists()).toBe(true)
  })

  it('mirrors the raw series onto data-points (comma-joined)', () => {
    const data = [3, 1, 4, 1, 5, 9, 2]
    const wrapper = mount(Sparkline, { props: { data } })
    expect(wrapper.find('svg').attributes('data-points')).toBe(data.join(','))
  })

  it('uses a normalized 0 0 100 20 viewBox with preserveAspectRatio="none"', () => {
    const wrapper = mount(Sparkline, { props: { data: [1, 2] } })
    const svg = wrapper.find('svg')
    expect(svg.attributes('viewBox')).toBe('0 0 100 20')
    expect(svg.attributes('preserveAspectRatio')).toBe('none')
  })

  it('spreads x evenly from 0 to 100 across the series', () => {
    const wrapper = mount(Sparkline, { props: { data: [0, 0, 0] } })
    const pts = wrapper.find('polyline').attributes('points')!.split(' ')
    expect(pts).toHaveLength(3)
    expect(pts[0].startsWith('0,')).toBe(true)
    expect(pts[2].startsWith('100,')).toBe(true)
  })

  it('stays finite (no NaN/Infinity) for a flat series — defensive range', () => {
    const wrapper = mount(Sparkline, { props: { data: [5, 5, 5, 5] } })
    const points = wrapper.find('polyline').attributes('points')!
    expect(points).not.toContain('NaN')
    expect(points).not.toContain('Infinity')
  })

  it('inherits colour via stroke="currentColor"', () => {
    const wrapper = mount(Sparkline, { props: { data: [1, 2, 3] } })
    expect(wrapper.find('polyline').attributes('stroke')).toBe('currentColor')
  })

  it('handles a single-point series without dividing by zero', () => {
    const wrapper = mount(Sparkline, { props: { data: [7] } })
    const points = wrapper.find('polyline').attributes('points')!
    expect(points).not.toContain('NaN')
    expect(points.startsWith('0,')).toBe(true)
  })
})
