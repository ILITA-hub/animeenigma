import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AboutBlock from '../AboutBlock.vue'

const mountWith = (config: object, variant?: string) =>
  mount(AboutBlock, {
    props: { config, ...(variant ? { variant } : {}) },
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

  it('default (no variant) renders quote variant with text', () => {
    const w = mountWith({ title: '// status', text: 'Quote text here' })
    expect(w.text()).toContain('Quote text here')
    // quote variant has the accent bar div with about-quote class
    expect(w.find('.about-quote').exists()).toBe(true)
  })

  it('renders vn variant with name-tag and text', () => {
    const w = mountWith({ title: 'TestUser', text: 'hello' }, 'vn')
    expect(w.text()).toContain('hello')
    expect(w.text()).toContain('TestUser')
    expect(w.find('.about-nametag').exists()).toBe(true)
  })

  it('renders terminal variant with prompt and whoami lines', () => {
    const w = mountWith({ title: '0neymik0', text: 'fan since 2009' }, 'terminal')
    // prompt arrow
    expect(w.text()).toContain('➜')
    // whoami command
    expect(w.text()).toContain('whoami')
    // user text content
    expect(w.text()).toContain('fan since 2009')
  })

  it('renders bio variant with avatar initials and text', () => {
    const w = mountWith({ title: 'Anime', text: 'bio text' }, 'bio')
    expect(w.text()).toContain('bio text')
    // bio has the av ring
    expect(w.find('.about-av-ring').exists()).toBe(true)
  })

  it('renders minimal variant with centered text', () => {
    const w = mountWith({ title: '0neymik0', text: 'minimal statement' }, 'minimal')
    expect(w.text()).toContain('minimal statement')
    expect(w.find('.about-min-big').exists()).toBe(true)
  })
})
