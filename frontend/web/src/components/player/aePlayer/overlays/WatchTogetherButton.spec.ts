import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WatchTogetherButton from './WatchTogetherButton.vue'

describe('WatchTogetherButton (WIP stub)', () => {
  it('renders the wt-wip data-test attribute', () => {
    const w = mount(WatchTogetherButton)
    expect(w.find('[data-test="wt-wip"]').exists()).toBe(true)
  })

  it('the button is disabled', () => {
    const w = mount(WatchTogetherButton)
    const btn = w.find('button')
    expect(btn.attributes('disabled')).toBeDefined()
  })

  it('emits nothing when clicked', async () => {
    const w = mount(WatchTogetherButton)
    const btn = w.find('button')
    await btn.trigger('click')
    // No emits at all
    expect(Object.keys(w.emitted()).length).toBe(0)
  })

  it('has a "coming soon" title tooltip', () => {
    const w = mount(WatchTogetherButton)
    const btn = w.find('button')
    expect(btn.attributes('title')).toContain('coming soon')
  })

  it('shows a WIP badge', () => {
    const w = mount(WatchTogetherButton)
    expect(w.text()).toContain('WIP')
  })
})
