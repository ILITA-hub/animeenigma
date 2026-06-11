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
  it('renders an eager proxied 2:3 poster with glow variant', () => {
    const w = mount(SpotlightPoster, {
      props: {
        posterUrl: 'https://shikimori.io/uploads/poster/animes/30/x.jpeg',
        widthClass: 'w-32',
        glow: 'pink',
        alt: 'NGE',
      },
    })
    const img = w.find('img')
    // EAGER on purpose (2026-06-11): the carousel mounts only the active
    // slide — lazy just delayed cached decodes and read as a reload.
    expect(img.attributes('loading')).toBeUndefined()
    expect(img.attributes('src')).toContain('image-proxy')
    expect(w.classes().join(' ')).toContain('aspect-[2/3]')
    expect(w.classes().join(' ')).toContain('shadow-pink-500/30')
    expect(w.classes().join(' ')).toContain('w-32')
  })

  it('cold load: shimmer + hidden img until @load; warm re-mount renders instantly', async () => {
    const url = 'https://shikimori.io/uploads/poster/animes/31/cold.jpeg'
    const w = mount(SpotlightPoster, { props: { posterUrl: url } })
    // Cold: skeleton visible, img transparent.
    expect(w.find('.skeleton-shimmer').exists()).toBe(true)
    expect(w.find('img').classes()).toContain('opacity-0')
    // Image finishes loading → shimmer gone, img visible, URL marked warm.
    await w.find('img').trigger('load')
    expect(w.find('.skeleton-shimmer').exists()).toBe(false)
    expect(w.find('img').classes()).toContain('opacity-100')
    // Re-mount with the same URL (carousel flip) → instant, no shimmer.
    const w2 = mount(SpotlightPoster, { props: { posterUrl: url } })
    expect(w2.find('.skeleton-shimmer').exists()).toBe(false)
    expect(w2.find('img').classes()).toContain('opacity-100')
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
