import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioGroup from './RadioGroup.vue'

const OPTIONS = [
  { value: '', label: 'Any' },
  { value: 'tv', label: 'TV' },
  { value: 'movie', label: 'Movie' },
]

describe('RadioGroup.vue (Reka RadioGroup, string v-model)', () => {
  it('renders a radiogroup with one radio per option', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    expect(w.find('[role="radiogroup"]').exists()).toBe(true)
    expect(w.findAll('[role="radio"]')).toHaveLength(3)
  })

  it('renders each option label', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    expect(w.text()).toContain('Any')
    expect(w.text()).toContain('TV')
    expect(w.text()).toContain('Movie')
  })

  it('supports an empty-string option value as selectable', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    const first = w.findAll('[role="radio"]')[0]
    expect(first.attributes('aria-checked')).toBe('true')
  })

  it('selecting an option emits update:modelValue with its value', async () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    await w.findAll('[role="radio"]')[1].trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['tv'])
  })

  it('reflects the checked option from modelValue', () => {
    const w = mount(RadioGroup, { props: { modelValue: 'movie', options: OPTIONS } })
    const radios = w.findAll('[role="radio"]')
    expect(radios[2].attributes('aria-checked')).toBe('true')
  })

  it('disables all items when disabled', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS, disabled: true } })
    for (const r of w.findAll('[role="radio"]')) {
      expect(r.attributes('disabled')).toBeDefined()
    }
  })
})
