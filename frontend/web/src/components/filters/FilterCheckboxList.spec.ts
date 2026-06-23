import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterCheckboxList from './FilterCheckboxList.vue'

const items = [
  { id: 'a', label: 'Action' },
  { id: 'c', label: 'Comedy' },
  { id: 'd', label: 'Drama', count: 7 },
]

describe('FilterCheckboxList', () => {
  it('renders one row per item with labels', () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    expect(w.text()).toContain('Action')
    expect(w.text()).toContain('Comedy')
    expect(w.text()).toContain('Drama')
  })

  it('shows the count when provided', () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    expect(w.text()).toContain('7')
  })

  // NOTE: `Checkbox` and `Input` are reka-ui-based primitives (Checkbox renders
  // <button role="checkbox">, NOT a native <input>), so interact via the
  // component's update:modelValue event — matching the existing pattern in
  // WatchlistFilters.spec.ts (findAllComponents({ name: 'Checkbox' })).
  it('emits update:selected adding a toggled id', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    await w.findAllComponents({ name: 'Checkbox' })[0].vm.$emit('update:modelValue', true)
    expect(w.emitted('update:selected')?.[0]).toEqual([['a']])
  })

  it('emits update:selected removing an already-selected id', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: ['a'] } })
    await w.findAllComponents({ name: 'Checkbox' })[0].vm.$emit('update:modelValue', false)
    expect(w.emitted('update:selected')?.[0]).toEqual([[]])
  })

  it('filters visible rows by the search query (case-insensitive)', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [], searchable: true } })
    await w.findComponent({ name: 'Input' }).vm.$emit('update:modelValue', 'com')
    expect(w.text()).toContain('Comedy')
    expect(w.text()).not.toContain('Action')
  })
})
