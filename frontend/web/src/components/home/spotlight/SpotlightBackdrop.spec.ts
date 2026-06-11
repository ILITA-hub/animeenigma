/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-01).
 *
 * Vitest snapshots + structural assertions for SpotlightBackdrop.vue.
 * One snapshot per (variant, accent) combination — 1 for poster-blur and
 * 3 for gradient-mesh × 3 brand-triad accents (DS alignment 2026-06-10).
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SpotlightBackdrop from './SpotlightBackdrop.vue'
import type { SpotlightAccent } from './tokens'

const ACCENTS: readonly SpotlightAccent[] = [
  'cyan',
  'pink',
  'violet',
] as const

describe('SpotlightBackdrop', () => {
  it('renders a blurred <img> in poster-blur variant', () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: {
        variant: 'poster-blur',
        posterUrl: 'https://example.test/poster.jpg',
        accent: 'cyan',
      },
    })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('https://example.test/poster.jpg')
    expect(img.attributes('aria-hidden')).toBe('true')
    expect(img.attributes('alt')).toBe('')
    // Blur values are part of the visual contract — assert literally.
    const style = img.attributes('style') ?? ''
    expect(style).toContain('blur(40px)')
    expect(style).toContain('saturate(1.2)')
    // The 0.4 alpha lives on the keyed wrapper (reroll crossfade, 2026-06-11)
    // so Transition enter/leave classes can animate it.
    expect(wrapper.find('.opacity-40').exists()).toBe(true)
  })

  it('swaps the blur <img> src when posterUrl changes (reroll crossfade)', async () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: {
        variant: 'poster-blur',
        posterUrl: 'https://example.test/old.jpg',
        accent: 'violet',
      },
    })
    await wrapper.setProps({ posterUrl: 'https://example.test/new.jpg' })
    await wrapper.vm.$nextTick()
    const srcs = wrapper.findAll('img').map((i) => i.attributes('src'))
    // The keyed-wrapper Transition crossfades: the NEW src must be mounted
    // (the old one may linger one frame while it fades out in browsers).
    expect(srcs).toContain('https://example.test/new.jpg')
  })

  it('falls back to gradient-mesh when poster-blur has no posterUrl', () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: { variant: 'poster-blur', posterUrl: '', accent: 'violet' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="spotlight-backdrop-mesh"]').exists()).toBe(true)
  })

  it('renders a gradient mesh in gradient-mesh variant', () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: { variant: 'gradient-mesh', accent: 'pink' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="spotlight-backdrop-mesh"]').exists()).toBe(true)
  })

  it.each(ACCENTS)(
    'gradient-mesh snapshot for accent=%s',
    (accent) => {
      const wrapper = mount(SpotlightBackdrop, {
        props: { variant: 'gradient-mesh', accent },
      })
      expect(wrapper.html()).toMatchSnapshot()
    },
  )

  it('poster-blur snapshot with mock URL', () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: {
        variant: 'poster-blur',
        posterUrl: 'https://shikimori.one/poster.jpg',
        accent: 'cyan',
      },
    })
    expect(wrapper.html()).toMatchSnapshot()
  })

  it('always renders pointer-events-none container + vignette overlay', () => {
    const wrapper = mount(SpotlightBackdrop, {
      props: { variant: 'gradient-mesh', accent: 'cyan' },
    })
    // The outermost decorative container must be inert to pointer + AT.
    expect(wrapper.find('.pointer-events-none').exists()).toBe(true)
    // Right-side vignette overlay must always be present so foreground text
    // is AA-readable on every variant.
    const vignettes = wrapper.findAll('.bg-gradient-to-r')
    expect(vignettes.length).toBeGreaterThanOrEqual(1)
  })

  it('uses a distinct mesh class per accent (no class collisions)', () => {
    const meshClasses = ACCENTS.map((accent) => {
      const wrapper = mount(SpotlightBackdrop, {
        props: { variant: 'gradient-mesh', accent },
      })
      return wrapper.find('[data-testid="spotlight-backdrop-mesh"]').classes().join(' ')
    })
    // All 6 accent class strings should be unique
    expect(new Set(meshClasses).size).toBe(ACCENTS.length)
  })
})
