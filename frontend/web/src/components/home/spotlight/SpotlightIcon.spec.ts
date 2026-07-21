/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-03).
 *
 * Verifies SpotlightIcon.vue renders every named icon via inline <svg>
 * and forwards the `class` attribute onto the SVG root.
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SpotlightIcon from './SpotlightIcon.vue'
import type { SpotlightIconName } from './tokens'

const ICONS: readonly SpotlightIconName[] = [
  'telegram',
  'code',
  'book',
  'sparkles',
  'chart',
  'pulse',
  'clock',
  'play',
  'shuffle',
  'wrench',
  'lightning',
  'star',
  'quote',
] as const

describe('SpotlightIcon', () => {
  it.each(ICONS)('renders an <svg> for icon "%s"', (name) => {
    const wrapper = mount(SpotlightIcon, { props: { name } })
    const svg = wrapper.find('svg')
    expect(svg.exists()).toBe(true)
    expect(svg.attributes('viewBox')).toBe('0 0 24 24')
    expect(svg.attributes('aria-hidden')).toBe('true')
  })

  it('forwards `class` attribute onto the SVG root', () => {
    const wrapper = mount(SpotlightIcon, {
      props: { name: 'sparkles' },
      attrs: { class: 'w-4 h-4 text-cyan-300' },
    })
    const svg = wrapper.find('svg')
    expect(svg.exists()).toBe(true)
    // attrs.class should propagate to svg root, not a wrapper element
    expect(svg.classes()).toEqual(expect.arrayContaining(['w-4', 'h-4', 'text-cyan-300']))
  })

  it('renders exactly one <svg> per icon (no duplicate v-if branches)', () => {
    const wrapper = mount(SpotlightIcon, { props: { name: 'play' } })
    expect(wrapper.findAll('svg')).toHaveLength(1)
  })

  it('renders distinct icon paths for two different names', () => {
    const a = mount(SpotlightIcon, { props: { name: 'telegram' } })
    const b = mount(SpotlightIcon, { props: { name: 'lightning' } })
    expect(a.html()).not.toBe(b.html())
  })

  // v1.1-polish Phase 06 — labeled mode for icons that carry semantic
  // meaning on their own (e.g. the Telegram brand mark in TelegramNewsCard's
  // header). When caller passes aria-label, the SVG becomes role="img"
  // and aria-hidden is dropped so screen readers announce it.
  it('drops aria-hidden + adds role=img + aria-label when labeled', () => {
    const wrapper = mount(SpotlightIcon, {
      props: { name: 'telegram' },
      attrs: { 'aria-label': 'Telegram' },
    })
    const svg = wrapper.find('svg')
    expect(svg.exists()).toBe(true)
    expect(svg.attributes('aria-label')).toBe('Telegram')
    expect(svg.attributes('role')).toBe('img')
    expect(svg.attributes('aria-hidden')).toBeUndefined()
  })

  it('keeps aria-hidden=true when caller omits aria-label (decorative default)', () => {
    const wrapper = mount(SpotlightIcon, { props: { name: 'play' } })
    const svg = wrapper.find('svg')
    expect(svg.attributes('aria-hidden')).toBe('true')
    expect(svg.attributes('role')).toBeUndefined()
    expect(svg.attributes('aria-label')).toBeUndefined()
  })
})
