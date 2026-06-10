import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ScoreDiamond from './ScoreDiamond.vue'

describe('ScoreDiamond', () => {
  it('renders the kite-shaped diamond path filled with currentColor', () => {
    const w = mount(ScoreDiamond)
    expect(w.element.tagName.toLowerCase()).toBe('svg')
    expect(w.attributes('fill')).toBe('currentColor')
    expect(w.find('path').attributes('d')).toBe('M12 2l9 10-9 10L3 12z')
  })

  it('is decorative for screen readers', () => {
    const w = mount(ScoreDiamond)
    expect(w.attributes('aria-hidden')).toBe('true')
  })

  it('passes size classes through to the svg root', () => {
    const w = mount(ScoreDiamond, { attrs: { class: 'size-3 text-cyan-400' } })
    expect(w.classes()).toContain('size-3')
    expect(w.classes()).toContain('text-cyan-400')
  })
})
