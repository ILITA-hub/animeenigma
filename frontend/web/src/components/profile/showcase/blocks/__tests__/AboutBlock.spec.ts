import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AboutBlock from '../AboutBlock.vue'

const mountWith = (config: object) =>
  mount(AboutBlock, {
    props: { config },
    global: { mocks: { $t: (k: string) => k } },
  })

describe('AboutBlock', () => {
  it('renders the title and text', () => {
    const w = mountWith({ title: 'Hello', text: 'I like anime' })
    expect(w.text()).toContain('Hello')
    expect(w.text()).toContain('I like anime')
  })

  it('renders nothing fatal when empty', () => {
    const w = mountWith({})
    expect(w.exists()).toBe(true)
  })
})
