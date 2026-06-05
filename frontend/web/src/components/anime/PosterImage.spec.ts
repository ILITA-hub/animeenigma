import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PosterImage from './PosterImage.vue'

describe('PosterImage', () => {
  it('shows the drift skeleton until the image loads', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/p.jpg', alt: 'P' } })
    expect(w.find('.sk-drift').exists()).toBe(true)
    expect(w.find('img').classes()).toContain('opacity-0')
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
    expect(w.find('img').classes()).toContain('opacity-100')
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
