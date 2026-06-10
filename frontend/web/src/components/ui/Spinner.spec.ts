import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { spinnerVariants } from './spinner-variants'
import Spinner from './Spinner.vue'

describe('spinnerVariants', () => {
  it('maps size to marker class', () => {
    expect(spinnerVariants({ size: 'sm' })).toContain('ae-spinner--sm')
    expect(spinnerVariants({ size: 'xl' })).toContain('ae-spinner--xl')
  })
  it('maps tone to marker class', () => {
    expect(spinnerVariants({ tone: 'mono' })).toContain('ae-spinner--mono')
  })
  it('defaults to md + signature', () => {
    const c = spinnerVariants({})
    expect(c).toContain('ae-spinner--md')
    expect(c).toContain('ae-spinner--signature')
  })
})

describe('Spinner.vue', () => {
  it('has role=status and a visually-hidden label', () => {
    const w = mount(Spinner, { props: { label: 'Загрузка' } })
    expect(w.attributes('role')).toBe('status')
    const sr = w.find('.sr-only')
    expect(sr.exists()).toBe(true)
    expect(sr.text()).toBe('Загрузка')
  })
  it('applies size + tone classes', () => {
    const w = mount(Spinner, { props: { size: 'lg', tone: 'mono' } })
    expect(w.classes()).toContain('ae-spinner--lg')
    expect(w.classes()).toContain('ae-spinner--mono')
  })
  it('defaults label to "Loading"', () => {
    const w = mount(Spinner)
    expect(w.find('.sr-only').text()).toBe('Loading')
  })
  it('passes extra classes through', () => {
    const w = mount(Spinner, { props: { class: 'mt-4' } })
    expect(w.classes()).toContain('mt-4')
  })
})
