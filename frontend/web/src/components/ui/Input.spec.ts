import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Input from './Input.vue'

describe('Input.vue', () => {
  it('renders exactly one <input>', () => {
    const w = mount(Input)
    expect(w.findAll('input')).toHaveLength(1)
  })

  it('v-model round-trips: setting value emits update:modelValue with the string', async () => {
    const w = mount(Input)
    await w.find('input').setValue('hello')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')![0]).toEqual(['hello'])
  })

  it('clearable + value renders clear button; clicking it emits update:modelValue with empty string', async () => {
    const w = mount(Input, { props: { clearable: true, modelValue: 'x' } })
    const btn = w.find('button')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual([''])
  })

  it('no clear button when clearable but value empty', () => {
    const w = mount(Input, { props: { clearable: true, modelValue: '' } })
    expect(w.find('button').exists()).toBe(false)
  })

  it('error prop renders a pink-400 error <p>', () => {
    const w = mount(Input, { props: { error: 'Bad' } })
    const p = w.find('p')
    expect(p.exists()).toBe(true)
    expect(p.classes()).toContain('text-pink-400')
    expect(p.text()).toContain('Bad')
  })

  it('hint (without error) renders a white/50 hint <p>', () => {
    const w = mount(Input, { props: { hint: 'Help' } })
    const p = w.find('p')
    expect(p.exists()).toBe(true)
    expect(p.classes()).toContain('text-white/50')
    expect(p.text()).toContain('Help')
  })

  it('#prefix slot renders in an absolute span and input gets pl-10', () => {
    const w = mount(Input, { slots: { prefix: '<i class="pfx" />' } })
    expect(w.find('.pfx').exists()).toBe(true)
    expect(w.find('input').classes()).toContain('pl-10')
  })

  it('#suffix slot renders and input gets pr-10', () => {
    const w = mount(Input, { slots: { suffix: '<i class="sfx" />' } })
    expect(w.find('.sfx').exists()).toBe(true)
    expect(w.find('input').classes()).toContain('pr-10')
  })

  it("size='md' input classes contain px-4 py-3 text-base rounded-xl", () => {
    const w = mount(Input, { props: { size: 'md' } })
    const cls = w.find('input').classes()
    expect(cls).toContain('px-4')
    expect(cls).toContain('py-3')
    expect(cls).toContain('text-base')
    expect(cls).toContain('rounded-xl')
  })

  it('error state input contains border-pink-500; non-error contains border-white/10', () => {
    const errored = mount(Input, { props: { error: 'x' } })
    expect(errored.find('input').classes()).toContain('border-pink-500')

    const clean = mount(Input)
    expect(clean.find('input').classes()).toContain('border-white/10')
  })

  it('renders the label when provided', () => {
    const w = mount(Input, { props: { label: 'Email' } })
    expect(w.find('label').exists()).toBe(true)
    expect(w.find('label').text()).toContain('Email')
  })
})
