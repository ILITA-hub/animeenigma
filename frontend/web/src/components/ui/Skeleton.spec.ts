import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Skeleton from './Skeleton.vue'

describe('Skeleton', () => {
  it('defaults to the pulse treatment', () => {
    const w = mount(Skeleton)
    expect(w.classes()).toContain('animate-pulse')
    expect(w.classes()).not.toContain('sk-drift')
  })

  it('renders the drift treatment when variant="drift"', () => {
    const w = mount(Skeleton, { props: { variant: 'drift' } })
    expect(w.classes()).toContain('sk-drift')
    expect(w.classes()).not.toContain('animate-pulse')
    expect(w.classes()).not.toContain('bg-white/10')
  })

  it('still applies rounded + custom className', () => {
    const w = mount(Skeleton, { props: { variant: 'drift', rounded: 'lg', className: 'h-20' } })
    expect(w.classes()).toContain('rounded-lg')
    expect(w.classes()).toContain('h-20')
  })
})
