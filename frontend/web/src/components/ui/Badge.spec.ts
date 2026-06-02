import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { badgeVariants } from './badge-variants'
import Badge from './Badge.vue'

describe('badgeVariants', () => {
  it('primary variant binds literal cyan utilities', () => {
    const c = badgeVariants({ variant: 'primary' })
    expect(c).toContain('bg-cyan-500/20')
    expect(c).toContain('text-cyan-400')
    // no accidental tokenization (Phase 2 rule: keep literal colors)
    expect(c).not.toContain('bg-primary')
  })

  it('default variant uses literal white utilities', () => {
    const c = badgeVariants({ variant: 'default' })
    expect(c).toContain('bg-white/10')
    expect(c).toContain('text-white/80')
  })

  it('secondary variant uses literal pink utilities', () => {
    const c = badgeVariants({ variant: 'secondary' })
    expect(c).toContain('bg-pink-500/20')
    expect(c).toContain('text-pink-400')
  })

  it('success variant uses literal emerald utilities', () => {
    const c = badgeVariants({ variant: 'success' })
    expect(c).toContain('bg-emerald-500/20')
    expect(c).toContain('text-emerald-400')
  })

  it('warning variant uses literal amber utilities', () => {
    const c = badgeVariants({ variant: 'warning' })
    expect(c).toContain('bg-amber-500/20')
    expect(c).toContain('text-amber-400')
  })

  it('rating variant uses black/amber + backdrop-blur', () => {
    const c = badgeVariants({ variant: 'rating' })
    expect(c).toContain('bg-black/60')
    expect(c).toContain('text-amber-400')
    expect(c).toContain('backdrop-blur-sm')
  })

  it('info variant uses literal purple utilities (not tokenized)', () => {
    const c = badgeVariants({ variant: 'info' })
    expect(c).toContain('bg-purple-500/20')
    expect(c).toContain('text-purple-400')
  })

  it('destructive variant uses literal red utilities', () => {
    const c = badgeVariants({ variant: 'destructive' })
    expect(c).toContain('bg-red-500/20')
    expect(c).toContain('text-red-400')
  })

  it('sizes map to expected utilities + literal radii', () => {
    expect(badgeVariants({ size: 'sm' })).toContain('px-2 py-0.5 text-xs rounded')
    expect(badgeVariants({ size: 'md' })).toContain('px-2.5 py-1 text-sm rounded-md')
    expect(badgeVariants({ size: 'lg' })).toContain('px-3 py-1.5 text-base rounded-lg')
  })

  it('default variants resolve to variant=default + size=md', () => {
    const c = badgeVariants({})
    expect(c).toContain('bg-white/10')
    expect(c).toContain('text-white/80')
    expect(c).toContain('px-2.5 py-1 text-sm rounded-md')
  })

  it('base includes inline-flex items-center font-medium', () => {
    const c = badgeVariants({})
    expect(c).toContain('inline-flex')
    expect(c).toContain('items-center')
    expect(c).toContain('font-medium')
  })
})

describe('Badge.vue', () => {
  it('renders a <span> element', () => {
    const w = mount(Badge)
    expect(w.element.tagName).toBe('SPAN')
  })

  it('renders default variant classes on the span', () => {
    const w = mount(Badge)
    expect(w.classes()).toContain('bg-white/10')
    expect(w.classes()).toContain('text-white/80')
  })

  it('applies variant + size props to rendered classes', () => {
    const w = mount(Badge, { props: { variant: 'primary', size: 'lg' } })
    expect(w.classes()).toContain('bg-cyan-500/20')
    expect(w.classes()).toContain('text-base')
    expect(w.classes()).toContain('rounded-lg')
  })

  it('renders default slot content', () => {
    const w = mount(Badge, { slots: { default: 'NEW' } })
    expect(w.text()).toContain('NEW')
  })

  it('renders #icon slot inside an mr-1 wrapper span when provided', () => {
    const w = mount(Badge, { slots: { icon: '<i class="my-icon" />' } })
    const wrapper = w.find('span.mr-1')
    expect(wrapper.exists()).toBe(true)
    expect(wrapper.find('.my-icon').exists()).toBe(true)
  })

  it('does not render the icon wrapper when no #icon slot', () => {
    const w = mount(Badge)
    expect(w.find('span.mr-1').exists()).toBe(false)
  })
})
