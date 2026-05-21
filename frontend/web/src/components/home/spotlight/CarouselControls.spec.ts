/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-02 / Task 2.
 *
 * Vitest spec for CarouselControls.vue. Verifies:
 *   1. Renders cardCount dot buttons + 2 chevron buttons
 *   2. aria-current="true" on active dot, "false" on siblings
 *   3. Click prev chevron emits 'prev' with no payload
 *   4. Click next chevron emits 'next' with no payload
 *   5. Click dot N emits 'goto' with 0-indexed payload N
 *   6. Dot aria-label receives 1-indexed slide number via t() params
 *   7. No raw text — every label flows through t()
 *
 * `vue-i18n` is stubbed with a fake t() that echoes the key (plus
 * JSON-encoded params if present) so we can assert against the key directly
 * without loading the actual locale JSON.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
  }),
}))

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import CarouselControls from './CarouselControls.vue'

describe('CarouselControls', () => {
  it('renders cardCount dot buttons + 2 chevron buttons', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cardCount: 4 },
    })

    // 2 chevrons + 4 dots = 6 total focusable buttons
    expect(wrapper.findAll('button').length).toBe(6)

    // aria-current is set on dots only — exactly cardCount instances
    expect(wrapper.findAll('[aria-current]').length).toBe(4)
  })

  it('marks the active dot with aria-current="true" and others with aria-current="false"', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 2, cardCount: 4 },
    })

    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(4)

    expect(dots[0].attributes('aria-current')).toBe('false')
    expect(dots[1].attributes('aria-current')).toBe('false')
    expect(dots[2].attributes('aria-current')).toBe('true')
    expect(dots[3].attributes('aria-current')).toBe('false')
  })

  it('emits prev when prev chevron clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cardCount: 3 },
    })

    const prev = wrapper.find('[aria-label="spotlight.prevSlide"]')
    expect(prev.exists()).toBe(true)
    await prev.trigger('click')

    expect(wrapper.emitted().prev).toBeTruthy()
    expect(wrapper.emitted().prev?.length).toBe(1)
    // No payload — `prev` is fire-and-forget
    expect(wrapper.emitted().prev?.[0]).toEqual([])
  })

  it('emits next when next chevron clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cardCount: 3 },
    })

    const next = wrapper.find('[aria-label="spotlight.nextSlide"]')
    expect(next.exists()).toBe(true)
    await next.trigger('click')

    expect(wrapper.emitted().next).toBeTruthy()
    expect(wrapper.emitted().next?.length).toBe(1)
    expect(wrapper.emitted().next?.[0]).toEqual([])
  })

  it('emits goto with 0-indexed payload when dot clicked', async () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cardCount: 4 },
    })

    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    expect(dots.length).toBe(4)

    // Click third dot — visually slide 3, internally idx 2 (0-indexed)
    await dots[2].trigger('click')

    const gotoEvents = wrapper.emitted('goto') as unknown as Array<[number]> | undefined
    expect(gotoEvents).toBeTruthy()
    expect(gotoEvents?.length).toBe(1)
    expect(gotoEvents?.[0]?.[0]).toBe(2)
  })

  it('passes slide number 1-indexed to t() for dot aria-label', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 0, cardCount: 3 },
    })

    const dots = wrapper.findAll('[data-testid="spotlight-dots"] button')
    // Third dot — should pass { n: 3 } (1-indexed for humans)
    const thirdLabel = dots[2].attributes('aria-label') ?? ''
    expect(thirdLabel).toContain('spotlight.goToSlide')
    expect(thirdLabel).toContain('"n":3')

    // First dot — { n: 1 }
    const firstLabel = dots[0].attributes('aria-label') ?? ''
    expect(firstLabel).toContain('"n":1')
  })

  it('has no raw text — every label flows through t()', () => {
    const wrapper = mount(CarouselControls, {
      props: { currentIndex: 1, cardCount: 3 },
    })

    const visibleText = wrapper.text().trim()
    // None of the English strings should appear as text nodes — they should
    // only exist as i18n keys consumed via aria-label.
    expect(visibleText).not.toMatch(/Previous/i)
    expect(visibleText).not.toMatch(/Next/i)
    expect(visibleText).not.toMatch(/Go to/i)
  })
})
