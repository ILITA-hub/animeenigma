import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CharacterImage from './CharacterImage.vue'

describe('CharacterImage', () => {
  it('overlays the drift skeleton on the always-visible image until it loads', async () => {
    const w = mount(CharacterImage, { props: { src: 'http://x/c.jpg', alt: 'C' } })
    expect(w.find('.sk-drift').exists()).toBe(true)
    // The img is NEVER opacity-hidden — the native progressive render must
    // stay visible under the translucent overlay as loading feedback.
    expect(w.find('img').classes()).not.toContain('opacity-0')
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
  })

  it('defaults to the 2/3 portrait ratio', () => {
    const w = mount(CharacterImage, { props: { src: 'x', alt: 'a' } })
    expect(w.classes()).toContain('aspect-[2/3]')
  })

  it('applies the taller 3/4 ratio when requested', () => {
    const w = mount(CharacterImage, { props: { src: 'x', alt: 'a', ratio: '3/4' } })
    expect(w.classes()).toContain('aspect-[3/4]')
  })

  it('renders the bottom scrim only when enabled', () => {
    const off = mount(CharacterImage, { props: { src: 'x', alt: 'a' } })
    expect(off.find('.bg-gradient-to-t').exists()).toBe(false)
    const on = mount(CharacterImage, { props: { src: 'x', alt: 'a', scrim: true } })
    expect(on.find('.bg-gradient-to-t').exists()).toBe(true)
  })

  it('renders default-slot overlay content (name/role/badges)', () => {
    const w = mount(CharacterImage, {
      props: { src: 'x', alt: 'a' },
      slots: { default: '<span class="ov-test">hi</span>' },
    })
    expect(w.find('.ov-test').exists()).toBe(true)
  })

  it('swaps to the fallback url once on error', async () => {
    const w = mount(CharacterImage, { props: { src: 'http://x/bad.jpg', alt: 'a' } })
    const img = w.find('img')
    await img.trigger('error')
    // dataset guard prevents an infinite error loop
    expect((img.element as HTMLImageElement).dataset.fallback).toBe('1')
  })
})
