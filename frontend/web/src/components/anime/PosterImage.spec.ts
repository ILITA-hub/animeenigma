import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PosterImage from './PosterImage.vue'

describe('PosterImage', () => {
  it('overlays the drift skeleton on the always-visible image until it loads', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/p.jpg', alt: 'P' } })
    expect(w.find('.sk-drift').exists()).toBe(true)
    // The img is NEVER opacity-hidden — the native progressive render must
    // stay visible under the translucent overlay as loading feedback.
    expect(w.find('img').classes()).not.toContain('opacity-0')
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
  })

  it('force-hides the skeleton once the sweep animation cap elapses, even if load never fires', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/p.jpg', alt: 'P' } })
    expect(w.find('.sk-drift').exists()).toBe(true)
    // The ::after sweep hits its iteration cap; animationend bubbles to the host.
    // No `load` event is ever fired — the skeleton must still disappear so a
    // stuck `loaded` can never leave a frozen streak over the poster.
    await w.find('.sk-drift').trigger('animationend')
    expect(w.find('.sk-drift').exists()).toBe(false)
  })

  it('hides the skeleton for an already-cached image whose load event does not re-fire', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/a.jpg', alt: 'P' } })
    const img = w.find('img').element as HTMLImageElement
    // Emulate a browser-cached, already-decoded image (Firefox SPA-nav revisit).
    Object.defineProperty(img, 'complete', { value: true, configurable: true })
    Object.defineProperty(img, 'naturalWidth', { value: 320, configurable: true })
    // Rebinding the src re-runs the post-flush cache sync, which must detect the
    // decoded pixels and mark it loaded without waiting on a `load` event.
    await w.setProps({ src: 'http://x/b.jpg' })
    expect(w.find('.sk-drift').exists()).toBe(false)
  })

  it('applies the requested aspect ratio', () => {
    const w = mount(PosterImage, { props: { src: 'x', alt: 'a', ratio: '16/9' } })
    expect(w.classes()).toContain('aspect-[16/9]')
  })

  it('renders default-slot overlay content', () => {
    const w = mount(PosterImage, {
      props: { src: 'x', alt: 'a' },
      slots: { default: '<span class="ov-test">hi</span>' },
    })
    expect(w.find('.ov-test').exists()).toBe(true)
  })

  it('swaps to the fallback url once on error', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/bad.jpg', alt: 'a' } })
    const img = w.find('img')
    await img.trigger('error')
    // dataset guard prevents an infinite error loop
    expect((img.element as HTMLImageElement).dataset.fallback).toBe('1')
  })
})
