import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PosterLightbox from './PosterLightbox.vue'

// Content is Teleported to <body>; stub teleport so it renders inline.
const mountPL = (open: boolean) =>
  mount(PosterLightbox, {
    props: { modelValue: open, src: 'http://x/p.jpg', alt: 'Poster' },
    global: { stubs: { teleport: true }, mocks: { $t: (k: string) => k } },
  })

describe('PosterLightbox', () => {
  it('contributes NOTHING to the DOM when closed (iOS 26 status-bar constraint)', () => {
    const w = mountPL(false)
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    expect(w.find('img').exists()).toBe(false)
  })

  it('renders the full-resolution image (original URL, not the resize proxy) when open', () => {
    const w = mountPL(true)
    expect(w.find('[role="dialog"]').exists()).toBe(true)
    const img = w.find('img')
    expect(img.attributes('src')).toBe('http://x/p.jpg')
    expect(img.attributes('alt')).toBe('Poster')
  })

  it('falls back to the backend image-proxy when the original errors', async () => {
    const w = mountPL(true)
    await w.find('img').trigger('error')
    expect(w.find('img').attributes('src')).toContain('/api/streaming/image-proxy')
  })

  it('close button emits update:modelValue=false', async () => {
    const w = mountPL(true)
    await w.find('button[aria-label="common.close"]').trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([[false]])
  })

  it('Escape closes via the document-level listener (works with focus on body)', async () => {
    const w = mountPL(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(w.emitted('update:modelValue')).toEqual([[false]])
    w.unmount()
  })
})
