/**
 * Workstream hero-spotlight — Vitest spec for CarouselControls.vue.
 *
 * Since the v1.1-polish crop fix, CarouselControls renders ONLY the two
 * prev/next chevron arrows (the labeled-pill dots were split out to
 * CarouselDots.vue — see CarouselDots.spec.ts). Verifies:
 *   1. Renders exactly 2 chevron buttons (no dots)
 *   2. Click prev chevron emits 'prev' with no payload
 *   3. Click next chevron emits 'next' with no payload
 *   4. No raw text — every label flows through t()
 *
 * `vue-i18n` is stubbed with a fake t() that echoes the key so we can
 * assert against the key directly without loading the actual locale JSON.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import CarouselControls from './CarouselControls.vue'

describe('CarouselControls', () => {
  it('renders exactly 2 chevron buttons and no dots', () => {
    const wrapper = mount(CarouselControls)

    expect(wrapper.findAll('button').length).toBe(2)
    expect(wrapper.find('[data-testid="spotlight-dots"]').exists()).toBe(false)
    // 2 chevron SVGs, nothing else.
    expect(wrapper.findAll('svg').length).toBe(2)
  })

  it('emits prev when prev chevron clicked', async () => {
    const wrapper = mount(CarouselControls)
    const prev = wrapper.find('[aria-label="spotlight.prevSlide"]')
    expect(prev.exists()).toBe(true)
    await prev.trigger('click')
    expect(wrapper.emitted().prev).toBeTruthy()
    expect(wrapper.emitted().prev?.length).toBe(1)
    expect(wrapper.emitted().prev?.[0]).toEqual([])
  })

  it('emits next when next chevron clicked', async () => {
    const wrapper = mount(CarouselControls)
    const next = wrapper.find('[aria-label="spotlight.nextSlide"]')
    expect(next.exists()).toBe(true)
    await next.trigger('click')
    expect(wrapper.emitted().next).toBeTruthy()
    expect(wrapper.emitted().next?.length).toBe(1)
    expect(wrapper.emitted().next?.[0]).toEqual([])
  })

  it('has no raw text — every label flows through t()', () => {
    const wrapper = mount(CarouselControls)
    const visibleText = wrapper.text().trim()
    expect(visibleText).not.toMatch(/Previous/i)
    expect(visibleText).not.toMatch(/Next/i)
  })
})
