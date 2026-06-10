import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { alertVariants, alertIconColor } from './alert-variants'
import Alert from './Alert.vue'

describe('alertVariants', () => {
  it('info binds info-soft bg + info border', () => {
    const c = alertVariants({ variant: 'info' })
    expect(c).toContain('bg-info-soft')
    expect(c).toContain('border-info/30')
  })
  it('destructive binds destructive-soft', () => {
    expect(alertVariants({ variant: 'destructive' })).toContain('bg-destructive-soft')
  })
  it('defaults to info', () => {
    expect(alertVariants({})).toContain('bg-info-soft')
  })
  it('icon color map covers all variants', () => {
    expect(alertIconColor.warning).toBe('text-warning')
    expect(alertIconColor.success).toBe('text-success')
  })
})

describe('Alert.vue', () => {
  it('has role=alert and default info classes', () => {
    const w = mount(Alert)
    expect(w.attributes('role')).toBe('alert')
    expect(w.classes()).toContain('bg-info-soft')
  })
  it('renders the title when provided', () => {
    const w = mount(Alert, { props: { title: 'Heads up' } })
    expect(w.text()).toContain('Heads up')
  })
  it('renders default slot body', () => {
    const w = mount(Alert, { slots: { default: 'Body text' } })
    expect(w.text()).toContain('Body text')
  })
  it('shows close button + emits dismiss only when dismissible', () => {
    const off = mount(Alert)
    expect(off.find('button[aria-label]').exists()).toBe(false)
    const on = mount(Alert, { props: { dismissible: true } })
    const btn = on.find('button[aria-label]')
    expect(btn.exists()).toBe(true)
    btn.trigger('click')
    expect(on.emitted('dismiss')).toBeTruthy()
  })
  it('uses #icon slot to override the default icon', () => {
    const w = mount(Alert, { slots: { icon: '<i class="custom-icon" />' } })
    expect(w.find('.custom-icon').exists()).toBe(true)
  })
})
