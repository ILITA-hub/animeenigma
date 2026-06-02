import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Checkbox from './Checkbox.vue'

// Checkbox uses boolean | 'indeterminate' v-model (Reka 2.x — NOT :checked).

describe('Checkbox.vue (Reka Checkbox, boolean v-model)', () => {
  it('renders a CheckboxRoot (role=checkbox)', () => {
    const w = mount(Checkbox, { props: { modelValue: false } })
    const root = w.find('[role="checkbox"]')
    expect(root.exists()).toBe(true)
  })

  it('toggling emits update:modelValue with true (boolean v-model)', async () => {
    const w = mount(Checkbox, { props: { modelValue: false } })
    await w.find('[role="checkbox"]').trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual([true])
  })

  it('checked-state root contains data-[state=checked]:bg-primary + border-input', () => {
    const w = mount(Checkbox, { props: { modelValue: true } })
    const cls = w.find('[role="checkbox"]').classes()
    expect(cls).toContain('data-[state=checked]:bg-primary')
    expect(cls).toContain('border-input')
  })

  it('renders a check icon (indicator) when checked', () => {
    const w = mount(Checkbox, { props: { modelValue: true } })
    // The indicator renders an inline SVG checkmark inside the root.
    expect(w.find('[role="checkbox"] svg').exists()).toBe(true)
  })

  it("accepts 'indeterminate' as a modelValue value", () => {
    const w = mount(Checkbox, { props: { modelValue: 'indeterminate' } })
    expect(w.find('[role="checkbox"]').exists()).toBe(true)
    const p = (w.vm as unknown as { $props: { modelValue: boolean | 'indeterminate' } }).$props
    expect(p.modelValue).toBe('indeterminate')
  })

  it('supports a disabled prop', () => {
    const w = mount(Checkbox, { props: { modelValue: false, disabled: true } })
    expect(w.find('[role="checkbox"]').attributes('disabled')).toBeDefined()
  })
})
