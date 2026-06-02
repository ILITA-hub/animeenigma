import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Card from './Card.vue'
import CardHeader from './CardHeader.vue'
import CardTitle from './CardTitle.vue'
import CardContent from './CardContent.vue'
import CardFooter from './CardFooter.vue'

describe('Card.vue root (glass preserved + prop API)', () => {
  it('default mount → glass-card + rounded-2xl + p-4 + block', () => {
    const c = mount(Card).classes()
    expect(c).toContain('glass-card')
    expect(c).toContain('rounded-2xl')
    expect(c).toContain('p-4')
    expect(c).toContain('block')
  })

  it('variant="elevated" → glass-elevated + rounded-2xl, no rounded-prop map', () => {
    const c = mount(Card, { props: { variant: 'elevated', rounded: 'md' } }).classes()
    expect(c).toContain('glass-elevated')
    expect(c).toContain('rounded-2xl')
    expect(c).not.toContain('rounded-md')
  })

  it('variant="interactive" → glass-card + card-hover + cursor-pointer', () => {
    const c = mount(Card, { props: { variant: 'interactive' } }).classes()
    expect(c).toContain('glass-card')
    expect(c).toContain('card-hover')
    expect(c).toContain('cursor-pointer')
  })

  it('padding="lg" → p-6; padding="none" → no p-* utility', () => {
    expect(mount(Card, { props: { padding: 'lg' } }).classes()).toContain('p-6')
    const none = mount(Card, { props: { padding: 'none' } }).classes()
    expect(none.some((c) => /^p-\d/.test(c))).toBe(false)
  })

  it('rounded="xl" with default variant → rounded-xl (not rounded-2xl)', () => {
    const c = mount(Card, { props: { rounded: 'xl' } }).classes()
    expect(c).toContain('rounded-xl')
    expect(c).not.toContain('rounded-2xl')
  })

  it('href → <a> with href; otherwise the `as` element', () => {
    const a = mount(Card, { props: { href: '/x' } })
    expect(a.element.tagName).toBe('A')
    expect(a.attributes('href')).toBe('/x')
    expect(mount(Card).element.tagName).toBe('DIV')
    expect(mount(Card, { props: { as: 'section' } }).element.tagName).toBe('SECTION')
  })
})

describe('Card subcomponents', () => {
  it('CardHeader base classes', () => {
    const c = mount(CardHeader).classes()
    expect(c).toContain('flex')
    expect(c).toContain('flex-col')
    expect(c).toContain('gap-y-1.5')
    expect(c).toContain('p-6')
  })

  it('CardTitle uses font-semibold (weight rule), renders h3', () => {
    const w = mount(CardTitle)
    expect(w.element.tagName).toBe('H3')
    expect(w.classes()).toContain('font-semibold')
  })

  it('CardContent base classes p-6 pt-0', () => {
    const c = mount(CardContent).classes()
    expect(c).toContain('p-6')
    expect(c).toContain('pt-0')
  })

  it('CardFooter base classes flex items-center p-6 pt-0', () => {
    const c = mount(CardFooter).classes()
    expect(c).toContain('flex')
    expect(c).toContain('items-center')
    expect(c).toContain('p-6')
    expect(c).toContain('pt-0')
  })

  it('subcomponents merge caller class via cn() (p-6 preserved alongside mt-2)', () => {
    const c = mount(CardContent, { props: { class: 'mt-2' } }).classes()
    expect(c).toContain('mt-2')
    expect(c).toContain('p-6')
  })
})
