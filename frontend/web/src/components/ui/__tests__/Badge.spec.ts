import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Badge from '../Badge.vue'

describe('Badge', () => {
  it('renders the inline (tinted) treatment by default', () => {
    const w = mount(Badge, { props: { variant: 'warning' }, slots: { default: '9.9' } })
    expect(w.classes()).toContain('bg-amber-500/20')
    expect(w.classes()).toContain('text-amber-400')
    expect(w.text()).toBe('9.9')
  })

  it('swaps to dark-glass when overlay is set, keeping the accent text', () => {
    const w = mount(Badge, { props: { variant: 'warning', overlay: true }, slots: { default: '9.9' } })
    // tailwind-merge drops the tinted bg in favour of the glass bg
    expect(w.classes()).not.toContain('bg-amber-500/20')
    expect(w.classes()).toContain('bg-black/[0.62]')
    expect(w.classes()).toContain('backdrop-blur-[6px]')
    expect(w.classes()).toContain('text-amber-400')
  })
})
