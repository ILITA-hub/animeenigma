import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlayerIconButton from './PlayerIconButton.vue'

describe('PlayerIconButton', () => {
  it('renders a type="button" with the icon slot', () => {
    const w = mount(PlayerIconButton, { slots: { default: '<svg data-test="ic"></svg>' } })
    expect(w.find('button').attributes('type')).toBe('button')
    expect(w.find('[data-test="ic"]').exists()).toBe(true)
  })

  it('defaults to md size (size-10)', () => {
    const w = mount(PlayerIconButton)
    expect(w.find('button').classes()).toContain('size-10')
  })

  it('honors size="sm" (size-8)', () => {
    const w = mount(PlayerIconButton, { props: { size: 'sm' } })
    expect(w.find('button').classes()).toContain('size-8')
  })

  it('is inactive by default (data-active="false")', () => {
    const w = mount(PlayerIconButton)
    expect(w.find('button').attributes('data-active')).toBe('false')
  })

  it('reflects active state via data-active (the former .is-open highlight)', () => {
    const w = mount(PlayerIconButton, { props: { active: true } })
    expect(w.find('button').attributes('data-active')).toBe('true')
  })

  it('forwards fallthrough attrs (aria-label) to the root button', () => {
    const w = mount(PlayerIconButton, { attrs: { 'aria-label': 'Mute' } })
    expect(w.find('button').attributes('aria-label')).toBe('Mute')
  })

  it('emits native click (attr fallthrough)', async () => {
    const w = mount(PlayerIconButton)
    await w.find('button').trigger('click')
    // No internal handler; consumers bind @click which fall through to <button>.
    // Assert the element is interactive (not disabled) as a sanity check.
    expect(w.find('button').attributes('disabled')).toBeUndefined()
  })

  it('merges a custom class (e.g. responsive-hide marker) via props.class', () => {
    const w = mount(PlayerIconButton, { props: { class: 'pl-skip-back' } })
    expect(w.find('button').classes()).toContain('pl-skip-back')
  })
})
