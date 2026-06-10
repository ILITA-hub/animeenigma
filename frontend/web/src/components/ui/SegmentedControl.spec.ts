import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SegmentedControl from './SegmentedControl.vue'

const options = [
  { value: 'day', label: 'Day' },
  { value: 'week', label: 'Week' },
  { value: 'month', label: 'Month' },
]

describe('SegmentedControl', () => {
  it('renders a radiogroup with one radio per option', () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'week', options } })
    expect(w.find('[role="radiogroup"]').exists()).toBe(true)
    expect(w.findAll('[role="radio"]').length).toBe(3)
  })

  it('marks the selected option as active (aria-checked + data-active)', () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'week', options } })
    const week = w.get('[data-value="week"]')
    expect(week.attributes('aria-checked')).toBe('true')
    expect(week.attributes('data-active')).toBe('true')
    const day = w.get('[data-value="day"]')
    expect(day.attributes('aria-checked')).toBe('false')
    expect(day.attributes('data-active')).toBe('false')
  })

  it('emits update:modelValue with the clicked value', async () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'week', options } })
    await w.get('[data-value="month"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['month'])
  })

  it('renders option text labels by default', () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'day', options } })
    expect(w.get('[data-value="day"]').text()).toBe('Day')
  })

  it('icon-only hides text and exposes label as aria-label + title', () => {
    const w = mount(SegmentedControl, {
      props: { modelValue: 'table', iconOnly: true, options: [
        { value: 'table', label: 'Table view' },
        { value: 'grid', label: 'Grid view' },
      ] },
    })
    const table = w.get('[data-value="table"]')
    expect(table.text()).toBe('')
    expect(table.attributes('aria-label')).toBe('Table view')
    expect(table.attributes('title')).toBe('Table view')
  })

  it('applies the group aria-label', () => {
    const w = mount(SegmentedControl, { props: { modelValue: 'day', options, ariaLabel: 'Calendar view' } })
    expect(w.get('[role="radiogroup"]').attributes('aria-label')).toBe('Calendar view')
  })
})
