// Spotlight UI primitive set (v4 lock, 2026-06-11) — contract tests for
// the spotlight-scoped DS primitives in components/home/spotlight/ui/.
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SpotlightTile from './SpotlightTile.vue'
import SpotlightPoster from './SpotlightPoster.vue'
import SpotlightChatBubble from './SpotlightChatBubble.vue'
import SpotlightStepper from './SpotlightStepper.vue'
import SpotlightProgress from './SpotlightProgress.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}))

describe('SpotlightTile', () => {
  it('renders glass tone by default with the DS surface classes', () => {
    const w = mount(SpotlightTile, { slots: { default: 'x' } })
    expect(w.classes().join(' ')).toContain('bg-white/5')
    expect(w.classes().join(' ')).toContain('border-white/10')
  })

  it('dark tone + interactive switch classes', () => {
    const w = mount(SpotlightTile, { props: { tone: 'dark', interactive: true, as: 'li' } })
    expect(w.element.tagName).toBe('LI')
    expect(w.classes().join(' ')).toContain('bg-black/30')
    expect(w.classes().join(' ')).toContain('hover:bg-white/10')
  })
})

describe('SpotlightPoster', () => {
  it('renders a lazy proxied 2:3 poster with glow variant', () => {
    const w = mount(SpotlightPoster, {
      props: {
        posterUrl: 'https://shikimori.io/uploads/poster/animes/30/x.jpeg',
        widthClass: 'w-32',
        glow: 'pink',
        alt: 'NGE',
      },
    })
    const img = w.find('img')
    expect(img.attributes('loading')).toBe('lazy')
    expect(img.attributes('src')).toContain('image-proxy')
    expect(w.classes().join(' ')).toContain('aspect-[2/3]')
    expect(w.classes().join(' ')).toContain('shadow-pink-500/30')
    expect(w.classes().join(' ')).toContain('w-32')
  })
})

describe('SpotlightChatBubble', () => {
  it('renders avatar, asymmetric bubble tail and time', () => {
    const w = mount(SpotlightChatBubble, {
      props: { time: '14:02' },
      slots: { default: '<p>post</p>' },
    })
    expect(w.find('.tg-avatar').exists()).toBe(true)
    expect(w.html()).toContain('rounded-bl-[4px]')
    expect(w.text()).toContain('14:02')
    expect(w.text()).toContain('post')
  })
})

describe('SpotlightStepper', () => {
  it('renders two dimmed watched chips and a pink NEW chip', () => {
    const w = mount(SpotlightStepper, { props: { lastWatched: 5, newEpisode: 6 } })
    expect(w.text()).toContain('epChip:{"n":4}')
    expect(w.text()).toContain('epChip:{"n":5}')
    expect(w.find('[data-testid="stepper-new"]').text()).toContain('epChipNew:{"n":6}')
    expect(w.find('[data-testid="stepper-new"]').classes().join(' ')).toContain('text-pink-400')
  })

  it('fresh viewer (lastWatched=0) gets only the NEW chip', () => {
    const w = mount(SpotlightStepper, { props: { lastWatched: 0, newEpisode: 1 } })
    expect(w.text()).not.toContain('✓')
    expect(w.find('[data-testid="stepper-new"]').exists()).toBe(true)
  })
})

describe('SpotlightProgress', () => {
  it('clamps percent and renders the accent fill + label', () => {
    const w = mount(SpotlightProgress, {
      props: { percent: 21, accent: 'pink', label: '5 ИЗ 24' },
    })
    const fill = w.find('[data-testid="progress-fill"]')
    expect(fill.attributes('style')).toContain('width: 21%')
    expect(fill.classes().join(' ')).toContain('from-pink-500')
    expect(w.text()).toContain('5 ИЗ 24')
  })

  it('clamps out-of-range percent', () => {
    const w = mount(SpotlightProgress, { props: { percent: 150 } })
    expect(w.find('[data-testid="progress-fill"]').attributes('style')).toContain('width: 100%')
  })
})
