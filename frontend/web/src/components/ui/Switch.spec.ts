import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Switch from './Switch.vue'

// Switch uses PLAIN boolean v-model (Reka 2.x — NOT v-model:checked). We assert
// the role=switch root, on/off token classes, the boolean emit, and disabled.

describe('Switch.vue (Reka Switch, boolean v-model)', () => {
  it('renders a SwitchRoot with role=switch', () => {
    const w = mount(Switch, { props: { modelValue: false } })
    const root = w.find('[role="switch"]')
    expect(root.exists()).toBe(true)
  })

  it('toggling emits update:modelValue with true (boolean v-model, not :checked)', async () => {
    const w = mount(Switch, { props: { modelValue: false } })
    await w.find('[role="switch"]').trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual([true])
  })

  it('on-state root contains bg-primary, off-state contains bg-white/10', () => {
    const off = mount(Switch, { props: { modelValue: false } })
    const cls = off.find('[role="switch"]').classes()
    // both data-variant utilities are always present (Reka toggles via data-state)
    expect(cls).toContain('data-[state=checked]:bg-primary')
    expect(cls).toContain('data-[state=unchecked]:bg-white/10')
  })

  it('thumb has bg-white rounded-full + a data-[state=checked] translate utility', () => {
    const w = mount(Switch, { props: { modelValue: true } })
    // The thumb is the inner span inside the switch button.
    const root = w.find('[role="switch"]')
    const thumb = root.find('span')
    expect(thumb.exists()).toBe(true)
    const cls = thumb.classes()
    expect(cls).toContain('bg-white')
    expect(cls).toContain('rounded-full')
    expect(cls).toContain('data-[state=checked]:translate-x-5')
  })

  it('supports a disabled prop (root reflects disabled)', () => {
    const w = mount(Switch, { props: { modelValue: false, disabled: true } })
    const root = w.find('[role="switch"]')
    expect(root.attributes('disabled')).toBeDefined()
  })
})
