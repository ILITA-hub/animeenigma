import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SpotlightCardShell from './SpotlightCardShell.vue'

describe('SpotlightCardShell', () => {
  it('renders kicker with accent class and icon', () => {
    const w = mount(SpotlightCardShell, {
      props: { accent: 'cyan', icon: 'sparkles', kicker: 'Рекомендуем сегодня' },
    })
    const kicker = w.find('p')
    expect(kicker.text()).toContain('Рекомендуем сегодня')
    expect(kicker.classes()).toContain('text-cyan-400')
    expect(kicker.find('svg').exists()).toBe(true)
  })

  it('maps pink and violet accents to their kicker classes', () => {
    const pink = mount(SpotlightCardShell, { props: { accent: 'pink', kicker: 'x' } })
    expect(pink.find('p').classes()).toContain('text-pink-400')
    const violet = mount(SpotlightCardShell, { props: { accent: 'violet', kicker: 'x' } })
    expect(violet.find('p').classes()).toContain('text-brand-violet')
  })

  it('hides the kicker row when no kicker label is given', () => {
    const w = mount(SpotlightCardShell, { props: { accent: 'cyan' } })
    expect(w.find('p').exists()).toBe(false)
  })

  it('renders default SpotlightBackdrop, suppressed by backdrop="none"', () => {
    const withBg = mount(SpotlightCardShell, { props: { accent: 'violet' } })
    expect(withBg.find('[data-testid="spotlight-backdrop-mesh"]').exists()).toBe(true)
    const without = mount(SpotlightCardShell, {
      props: { accent: 'violet', backdrop: 'none' },
      slots: { background: '<div data-testid="custom-bg" />' },
    })
    expect(without.find('[data-testid="spotlight-backdrop-mesh"]').exists()).toBe(false)
    expect(without.find('[data-testid="custom-bg"]').exists()).toBe(true)
  })

  it('pins the CTA row bottom-left (mt-auto) in start mode', () => {
    const w = mount(SpotlightCardShell, {
      props: { accent: 'cyan' },
      slots: { cta: '<button>go</button>' },
    })
    const row = w.find('.mt-auto')
    expect(row.exists()).toBe(true)
    expect(row.text()).toContain('go')
  })

  it('bottom-anchors content (justify-end, no mt-auto) in end mode', () => {
    const w = mount(SpotlightCardShell, {
      props: { accent: 'cyan', justify: 'end' },
      slots: { cta: '<button>go</button>' },
    })
    expect(w.find('.justify-end').exists()).toBe(true)
    expect(w.find('.mt-auto').exists()).toBe(false)
  })

  it('uses the DS padding scale and a single <article> root', () => {
    const w = mount(SpotlightCardShell, { props: { accent: 'cyan' } })
    expect(w.element.tagName).toBe('ARTICLE')
    const col = w.find('.p-4')
    expect(col.classes()).toEqual(expect.arrayContaining(['md:p-6', 'lg:p-8']))
  })
})
