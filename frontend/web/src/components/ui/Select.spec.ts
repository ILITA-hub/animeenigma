import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Select, { type SelectOption } from './Select.vue'

// NOTE on jsdom portal limitation (RESEARCH Pitfall 6): Reka SelectContent
// portals to body and jsdom won't fully render it. We assert on the trigger +
// the emitted events (driven through SelectRoot's @update:modelValue bridge)
// rather than on portaled <SelectItem> DOM. Interaction correctness (keyboard,
// popper position/width) is covered by the in-browser gate, not jsdom.

const options: SelectOption[] = [
  { value: 'a', label: 'A' },
  { value: 'b', label: 'B' },
]

describe('Select.vue', () => {
  it('renders a trigger showing the selected option label', () => {
    const w = mount(Select, { props: { options, modelValue: 'a' } })
    const trigger = w.find('button[aria-haspopup="listbox"]')
    expect(trigger.exists()).toBe(true)
    expect(trigger.text()).toContain('A')
  })

  it('selecting an option emits BOTH update:modelValue AND change with the value', async () => {
    const w = mount(Select, { props: { options, modelValue: 'a' } })
    // Drive the API bridge directly (do not depend on portaled SelectItem DOM).
    ;(w.vm as unknown as { onSelect: (v: string | number) => void }).onSelect('b')
    await w.vm.$nextTick()

    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['b'])
    expect(w.emitted('change')).toBeTruthy()
    expect(w.emitted('change')!.at(-1)).toEqual(['b'])
  })

  it('with no modelValue, the trigger shows the placeholder text in the white/30 placeholder class', () => {
    const w = mount(Select, { props: { options, placeholder: 'Pick one' } })
    const placeholder = w.find('.text-white\\/30')
    expect(placeholder.exists()).toBe(true)
    expect(placeholder.text()).toContain('Pick one')
  })

  it("size='md' trigger classes contain px-4 py-3 text-base rounded-xl", () => {
    const w = mount(Select, { props: { options, modelValue: 'a', size: 'md' } })
    const trigger = w.find('button[aria-haspopup="listbox"]')
    const cls = trigger.classes()
    expect(cls).toContain('px-4')
    expect(cls).toContain('py-3')
    expect(cls).toContain('text-base')
    expect(cls).toContain('rounded-xl')
  })

  it("size='xs' trigger classes contain px-2 py-1 text-xs rounded-lg", () => {
    const w = mount(Select, { props: { options, modelValue: 'a', size: 'xs' } })
    const trigger = w.find('button[aria-haspopup="listbox"]')
    const cls = trigger.classes()
    expect(cls).toContain('px-2')
    expect(cls).toContain('py-1')
    expect(cls).toContain('text-xs')
    expect(cls).toContain('rounded-lg')
  })

  it('renders the label when provided', () => {
    const w = mount(Select, { props: { options, modelValue: 'a', label: 'Sort by' } })
    expect(w.find('label').exists()).toBe(true)
    expect(w.find('label').text()).toContain('Sort by')
  })

  it('exports the SelectOption type (compile-time check)', () => {
    const o: SelectOption = { value: 1, label: 'one' }
    expect(o.value).toBe(1)
    expect(o.label).toBe('one')
  })
})
