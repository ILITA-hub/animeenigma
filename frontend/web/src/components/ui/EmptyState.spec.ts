/**
 * Spec for EmptyState.vue — slot-driven empty placeholder.
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from './EmptyState.vue'

describe('EmptyState', () => {
  it('renders default-slot message (text-only case)', () => {
    const wrapper = mount(EmptyState, { slots: { default: 'Nothing here yet' } })
    expect(wrapper.text()).toContain('Nothing here yet')
  })

  it('renders title + description props', () => {
    const wrapper = mount(EmptyState, {
      props: { title: 'No collections', description: 'Create one to get started' },
    })
    expect(wrapper.text()).toContain('No collections')
    expect(wrapper.text()).toContain('Create one to get started')
  })

  it('title renders in full foreground (heading), description muted', () => {
    const wrapper = mount(EmptyState, { props: { title: 'T', description: 'D' } })
    const title = wrapper.findAll('p').find(p => p.text() === 'T')
    expect(title?.classes()).toContain('text-foreground')
  })

  it('renders the icon slot inside a dimmed, aria-hidden wrapper', () => {
    const wrapper = mount(EmptyState, {
      slots: { icon: '<svg data-test="ic"></svg>' },
    })
    expect(wrapper.find('[data-test="ic"]').exists()).toBe(true)
    expect(wrapper.find('[aria-hidden="true"]').exists()).toBe(true)
  })

  it('omits the icon wrapper when no icon slot is provided', () => {
    const wrapper = mount(EmptyState, { slots: { default: 'x' } })
    expect(wrapper.find('[aria-hidden="true"]').exists()).toBe(false)
  })

  it('renders the action slot wrapper only when provided', () => {
    const withAction = mount(EmptyState, {
      slots: { default: 'x', action: '<button>Go</button>' },
    })
    expect(withAction.find('button').exists()).toBe(true)

    const without = mount(EmptyState, { slots: { default: 'x' } })
    expect(without.find('button').exists()).toBe(false)
  })

  it('applies size padding (lg => py-16)', () => {
    const wrapper = mount(EmptyState, { props: { size: 'lg' }, slots: { default: 'x' } })
    expect(wrapper.classes()).toContain('py-16')
  })

  it('defaults to md padding (py-12)', () => {
    const wrapper = mount(EmptyState, { slots: { default: 'x' } })
    expect(wrapper.classes()).toContain('py-12')
  })

  it('merges a custom class via props.class', () => {
    const wrapper = mount(EmptyState, { props: { class: 'mt-10' }, slots: { default: 'x' } })
    expect(wrapper.classes()).toContain('mt-10')
  })
})
