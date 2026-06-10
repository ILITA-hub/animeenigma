import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BufferingOverlay from './BufferingOverlay.vue'

describe('BufferingOverlay', () => {
  it('renders the spinner when visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(true)
    expect(w.find('.pl-buffering-ring').exists()).toBe(true)
  })

  it('renders nothing when not visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: false } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(false)
  })

  it('is announced as status for screen readers', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[data-test="buffering-overlay"]').attributes('role')).toBe('status')
  })
})
