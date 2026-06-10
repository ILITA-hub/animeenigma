import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import LoadingState from './LoadingState.vue'
import Spinner from './Spinner.vue'

describe('LoadingState.vue', () => {
  it('renders a Spinner', () => {
    const w = mount(LoadingState)
    expect(w.findComponent(Spinner).exists()).toBe(true)
  })
  it('shows the label when provided', () => {
    const w = mount(LoadingState, { props: { label: 'Loading episodes…' } })
    expect(w.text()).toContain('Loading episodes…')
  })
  it('omits the label element when no label', () => {
    const w = mount(LoadingState)
    expect(w.find('[data-testid="loadingstate-label"]').exists()).toBe(false)
  })
  it('forwards size + tone to the Spinner', () => {
    const w = mount(LoadingState, { props: { size: 'md', tone: 'mono' } })
    const sp = w.findComponent(Spinner)
    expect(sp.props('size')).toBe('md')
    expect(sp.props('tone')).toBe('mono')
  })
  it('is centered (flex-col items-center)', () => {
    const w = mount(LoadingState)
    expect(w.classes()).toContain('flex-col')
    expect(w.classes()).toContain('items-center')
  })
})
