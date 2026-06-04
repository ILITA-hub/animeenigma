import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { buttonVariants } from './button-variants'
import Button from './Button.vue'

describe('buttonVariants', () => {
  it('default variant binds cyan token + radius + hover token', () => {
    const c = buttonVariants({ variant: 'default' })
    expect(c).toContain('bg-primary')
    expect(c).toContain('rounded-xl')
    expect(c).toContain('hover:bg-brand-cyan')
  })

  it('brand variant binds pink token + foreground', () => {
    const c = buttonVariants({ variant: 'brand' })
    expect(c).toContain('bg-brand-pink')
    expect(c).toContain('text-brand-pink-foreground')
  })

  it('ghost variant uses literal white utilities, not bg-accent', () => {
    const c = buttonVariants({ variant: 'ghost' })
    expect(c).toContain('bg-white/5')
    expect(c).toContain('hover:bg-white/10')
    expect(c).not.toContain('bg-accent')
  })

  it('outline variant uses cyan-400 text + border', () => {
    const c = buttonVariants({ variant: 'outline' })
    expect(c).toContain('text-cyan-400')
    expect(c).toContain('border-cyan-400/50')
  })

  it('destructive variant binds destructive tokens', () => {
    const c = buttonVariants({ variant: 'destructive' })
    expect(c).toContain('bg-destructive')
    expect(c).toContain('text-destructive-foreground')
  })

  it('base uses ring-ring token (not ring-cyan-400) + ring-2', () => {
    const c = buttonVariants({ variant: 'default' })
    expect(c).toContain('ring-ring')
    expect(c).not.toContain('ring-cyan-400')
    expect(c).toContain('focus-visible:ring-2')
  })

  it('sizes map to expected utilities', () => {
    expect(buttonVariants({ size: 'sm' })).toContain('px-3 py-1.5 text-sm')
    expect(buttonVariants({ size: 'md' })).toContain('px-6 py-3 text-base')
    expect(buttonVariants({ size: 'lg' })).toContain('px-8 py-4 text-lg')
    expect(buttonVariants({ size: 'icon' })).toContain('h-10 w-10 p-0')
  })

  it('soft variant: quiet filled, no glow, no border', () => {
    const c = buttonVariants({ variant: 'soft' })
    expect(c).toContain('bg-white/10')
    expect(c).toContain('hover:bg-white/20')
    expect(c).not.toContain('border')
    expect(c).not.toContain('shadow-glow')
  })

  it('link variant: bare text, brand-cyan, padding zeroed', () => {
    const c = buttonVariants({ variant: 'link' })
    expect(c).toContain('text-cyan-400')
    expect(c).toContain('hover:underline')
    expect(c).toContain('bg-transparent')
    expect(c).toContain('px-0!')
    expect(c).not.toContain('active:scale')
  })

  it('new sizes map to expected utilities', () => {
    expect(buttonVariants({ size: 'xs' })).toContain('px-2 py-1 text-xs')
    expect(buttonVariants({ size: 'icon-sm' })).toContain('h-8 w-8 p-0')
  })

  it('default/brand glow uses the shadow-glow token, NOT raw rgba', () => {
    const d = buttonVariants({ variant: 'default' })
    expect(d).toContain('hover:shadow-glow-cyan')
    expect(d).not.toContain('rgba(0,212,255')
    const b = buttonVariants({ variant: 'brand' })
    expect(b).toContain('hover:shadow-glow-pink')
    expect(b).not.toContain('rgba(255,45,124')
  })

  it('legacy primary/secondary aliases still mirror default/brand glow tokens', () => {
    expect(buttonVariants({ variant: 'primary' })).toContain('hover:shadow-glow-cyan')
    expect(buttonVariants({ variant: 'secondary' })).toContain('hover:shadow-glow-pink')
  })
})

describe('Button.vue back-compat', () => {
  it('legacy variant="primary" renders identical to default (bg-primary)', () => {
    const w = mount(Button, { props: { variant: 'primary' } })
    expect(w.classes()).toContain('bg-primary')
  })

  it('legacy variant="secondary" renders identical to brand (bg-brand-pink)', () => {
    const w = mount(Button, { props: { variant: 'secondary' } })
    expect(w.classes()).toContain('bg-brand-pink')
  })

  it('renders a <button> by default and an <a> with href set', () => {
    const btn = mount(Button)
    expect(btn.element.tagName).toBe('BUTTON')
    const a = mount(Button, { props: { href: '/x' } })
    expect(a.element.tagName).toBe('A')
    expect(a.attributes('href')).toBe('/x')
  })

  it('click fires; full-width adds w-full; loading shows spinner + disables; #icon slot renders when not loading', () => {
    const click = mount(Button)
    click.trigger('click')
    expect(click.emitted('click')).toBeTruthy()

    const fw = mount(Button, { props: { fullWidth: true } })
    expect(fw.classes()).toContain('w-full')

    const loading = mount(Button, { props: { loading: true } })
    expect(loading.find('span.animate-spin').exists()).toBe(true)
    expect(loading.attributes('disabled')).toBeDefined()

    const withIcon = mount(Button, { slots: { icon: '<i class="my-icon" />' } })
    expect(withIcon.find('.my-icon').exists()).toBe(true)

    const loadingWithIcon = mount(Button, { props: { loading: true }, slots: { icon: '<i class="my-icon" />' } })
    expect(loadingWithIcon.find('.my-icon').exists()).toBe(false)
  })
})
