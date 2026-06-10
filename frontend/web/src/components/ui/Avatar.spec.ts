import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { avatarVariants, avatarInitials } from './avatar-variants'
import Avatar from './Avatar.vue'

describe('avatar helpers', () => {
  it('size maps to size-* class', () => {
    expect(avatarVariants({ size: 'lg' })).toContain('size-12')
  })
  it('initials: two words → 2 letters', () => {
    expect(avatarInitials('Alice Brown')).toBe('AB')
  })
  it('initials: one word → 1 letter', () => {
    expect(avatarInitials('Yuki')).toBe('Y')
  })
  it('initials: empty → ?', () => {
    expect(avatarInitials('')).toBe('?')
    expect(avatarInitials(undefined)).toBe('?')
  })
})

describe('Avatar.vue', () => {
  it('renders <img> when src is set', () => {
    const w = mount(Avatar, { props: { src: 'https://x/y.png', name: 'Al B' } })
    const img = w.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('https://x/y.png')
  })
  it('falls back to initials on image error', async () => {
    const w = mount(Avatar, { props: { src: 'https://x/broken.png', name: 'Al B' } })
    await w.find('img').trigger('error')
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('AB')
  })
  it('renders initials when no src', () => {
    const w = mount(Avatar, { props: { name: 'Static Virtual' } })
    expect(w.text()).toContain('SV')
  })
  it('renders presence dot with the right color', () => {
    const w = mount(Avatar, { props: { name: 'A', status: 'online' } })
    expect(w.find('.bg-success').exists()).toBe(true)
  })
  it('omits the dot when no status', () => {
    const w = mount(Avatar, { props: { name: 'A' } })
    expect(w.find('.bg-success').exists()).toBe(false)
  })
})
