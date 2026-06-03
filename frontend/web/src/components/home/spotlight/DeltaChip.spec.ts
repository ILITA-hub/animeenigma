/**
 * Workstream hero-spotlight — v1.1-polish Phase 08 (platform-stats-refactor).
 *
 * Vitest spec for DeltaChip.vue. Verifies the ↑/↓/— symbol + colour + the
 * in-component percent computation across the boundary cases.
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'

import DeltaChip from './DeltaChip.vue'

describe('DeltaChip', () => {
  it('renders ↑ and green when current > previous', () => {
    const wrapper = mount(DeltaChip, { props: { current: 120, previous: 100 } })
    expect(wrapper.text()).toContain('↑')
    expect(wrapper.text()).toContain('20%')
    expect(wrapper.html()).toContain('text-success')
  })

  it('renders ↓ and red when current < previous', () => {
    const wrapper = mount(DeltaChip, { props: { current: 50, previous: 100 } })
    expect(wrapper.text()).toContain('↓')
    expect(wrapper.text()).toContain('50%')
    expect(wrapper.html()).toContain('text-destructive')
  })

  it('renders — and gray when current === previous', () => {
    const wrapper = mount(DeltaChip, { props: { current: 100, previous: 100 } })
    expect(wrapper.text()).toContain('—')
    expect(wrapper.html()).toContain('text-muted-foreground')
  })

  it('renders — and gray with empty pct when previous is null (no baseline)', () => {
    const wrapper = mount(DeltaChip, { props: { current: 42, previous: null } })
    expect(wrapper.text()).toContain('—')
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.html()).toContain('text-muted-foreground')
  })

  it('treats previous <= 0 as no baseline (—, no division)', () => {
    const wrapper = mount(DeltaChip, { props: { current: 5, previous: 0 } })
    expect(wrapper.text()).toContain('—')
    expect(wrapper.text()).not.toContain('NaN')
    expect(wrapper.text()).not.toContain('Infinity')
  })

  it('rounds the percent to a whole number', () => {
    // (137 - 100) / 100 = 0.37 → 37%
    const wrapper = mount(DeltaChip, { props: { current: 137, previous: 100 } })
    expect(wrapper.text()).toContain('37%')
  })

  it('uses only font-semibold weight (UI-SPEC typography contract)', () => {
    const wrapper = mount(DeltaChip, { props: { current: 10, previous: 5 } })
    const html = wrapper.html()
    expect(html).toContain('font-semibold')
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
  })
})
