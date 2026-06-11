import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Stepper from './Stepper.vue'

describe('Stepper (ui primitive)', () => {
  it('renders the current value and suffix', () => {
    const w = mount(Stepper, { props: { modelValue: 1.5, step: 0.1, suffix: 's' } })
    expect((w.find('[data-test="stepper-input"]').element as HTMLInputElement).value).toBe('1.5')
    expect(w.text()).toContain('s')
  })

  it('increments by step on +', async () => {
    const w = mount(Stepper, { props: { modelValue: 0.2, step: 0.1 } })
    await w.find('[data-test="stepper-inc"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([0.3])
  })

  it('decrements by step on − without float drift', async () => {
    const w = mount(Stepper, { props: { modelValue: 0.3, step: 0.1 } })
    await w.find('[data-test="stepper-dec"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([0.2])
  })

  it('clamps to min and max', async () => {
    const w = mount(Stepper, { props: { modelValue: 0, step: 1, min: 0, max: 2 } })
    await w.find('[data-test="stepper-dec"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([0])
    await w.setProps({ modelValue: 2 })
    await w.find('[data-test="stepper-inc"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[1]).toEqual([2])
  })

  it('normalizes manual input to the step precision', async () => {
    const w = mount(Stepper, { props: { modelValue: 0, step: 0.1 } })
    const input = w.find('[data-test="stepper-input"]')
    ;(input.element as HTMLInputElement).value = '1.2345'
    await input.trigger('change')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([1.2])
  })

  it('labels the controls accessibly', () => {
    const w = mount(Stepper, { props: { modelValue: 0, label: 'offset' } })
    expect(w.find('[data-test="stepper-dec"]').attributes('aria-label')).toBe('Decrease offset')
    expect(w.find('[data-test="stepper-inc"]').attributes('aria-label')).toBe('Increase offset')
  })
})
