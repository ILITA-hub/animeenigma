import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BufferingOverlay from './BufferingOverlay.vue'
import Spinner from '@/components/ui/Spinner.vue'

describe('BufferingOverlay', () => {
  it('renders the design-system Spinner when visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(true)
    expect(w.findComponent(Spinner).exists()).toBe(true)
  })

  it('renders nothing when not visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: false } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(false)
  })

  it('is announced as status for screen readers', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[role="status"]').exists()).toBe(true)
    expect(w.text()).toContain('Buffering')
  })
})
