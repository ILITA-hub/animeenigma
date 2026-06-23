import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterYearRange from './FilterYearRange.vue'

// Select is a reka-ui primitive (portaled options); interact via the
// component's update:modelValue event and assert via its props.
function mountWith(props: Record<string, unknown> = {}) {
  return mount(FilterYearRange, {
    props: { min: null, max: null, floorYear: 2010, ceilYear: 2012, ...props },
  })
}

describe('FilterYearRange', () => {
  it('builds descending year options between ceil and floor plus an ANY sentinel', () => {
    const w = mountWith()
    const opts = w.findAllComponents({ name: 'Select' })[0].props('options') as { value: string; label: string }[]
    expect(opts.map((o) => o.value)).toEqual(['any', '2012', '2011', '2010'])
  })

  it('shows the ANY sentinel when a bound is null', () => {
    const w = mountWith({ min: null, max: 2011 })
    expect(w.findAllComponents({ name: 'Select' })[0].props('modelValue')).toBe('any')
    expect(w.findAllComponents({ name: 'Select' })[1].props('modelValue')).toBe('2011')
  })

  it('emits a numeric update:min when a year is chosen', async () => {
    const w = mountWith()
    await w.findAllComponents({ name: 'Select' })[0].vm.$emit('update:modelValue', '2011')
    expect(w.emitted('update:min')?.[0]).toEqual([2011])
  })

  it('emits null update:max when ANY is chosen', async () => {
    const w = mountWith({ max: 2012 })
    await w.findAllComponents({ name: 'Select' })[1].vm.$emit('update:modelValue', 'any')
    expect(w.emitted('update:max')?.[0]).toEqual([null])
  })

  it('bumps max up to keep min <= max', async () => {
    const w = mountWith({ min: null, max: 2010 })
    await w.findAllComponents({ name: 'Select' })[0].vm.$emit('update:modelValue', '2012')
    expect(w.emitted('update:min')?.[0]).toEqual([2012])
    expect(w.emitted('update:max')?.[0]).toEqual([2012])
  })
})
