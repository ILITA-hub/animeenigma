/**
 * Workstream hero-spotlight — v4 D-4 lock (2026-06-11): hero post +
 * Telegram chat bubbles. Contract:
 *
 *   1. Single-root <article> (Transition out-in safety).
 *   2. posts[0] renders as the hero tile (photo with overlay date badge,
 *      or a ✈ vignette when image_url is absent) + «Открыть пост» anchor
 *      with rel="noopener noreferrer" (T-03-18).
 *   3. posts[1..2] render as SpotlightChatBubble items.
 *   4. «Подписаться на канал» ghost anchor → https://t.me/anime_enigma.
 *   5. @anime_enigma attribution in the kicker-extra.
 */
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

import TelegramNewsCard from './TelegramNewsCard.vue'

function post(i: number, overrides: Record<string, unknown> = {}) {
  return {
    title: `Post ${i}`,
    excerpt: `Excerpt ${i}`,
    link: `https://t.me/anime_enigma/${i}`,
    date: new Date(Date.now() - i * 86_400_000).toISOString(),
    ...overrides,
  }
}

function mountCard(posts: unknown[]) {
  return mount(TelegramNewsCard, {
    props: { data: { posts } } as unknown as InstanceType<typeof TelegramNewsCard>['$props'],
  })
}

describe('TelegramNewsCard (v4 D-4 hero + bubbles)', () => {
  it('has a single <article> root element (Transition out-in safety)', () => {
    const wrapper = mountCard([post(0)])
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('renders the latest post as the hero tile with the open-post anchor', () => {
    const wrapper = mountCard([post(0, { image_url: 'https://cdn.example/img.jpg' }), post(1)])
    const heroTile = wrapper.find('[data-testid="tg-hero-post"]')
    expect(heroTile.exists()).toBe(true)
    expect(heroTile.text()).toContain('Post 0')
    const img = heroTile.find('img')
    expect(img.attributes('src')).toBe('https://cdn.example/img.jpg')
    const anchor = heroTile.find('a[target="_blank"]')
    expect(anchor.attributes('rel')).toBe('noopener noreferrer')
    expect(anchor.text()).toContain('spotlight.telegramNews.openCta')
  })

  it('photoless hero post renders the ✈ vignette instead of an img', () => {
    const wrapper = mountCard([post(0)])
    const heroTile = wrapper.find('[data-testid="tg-hero-post"]')
    expect(heroTile.find('img').exists()).toBe(false)
    expect(heroTile.find('svg').exists()).toBe(true)
  })

  it('renders posts[1..2] as chat bubbles (capped at 2)', () => {
    const wrapper = mountCard([post(0), post(1), post(2), post(3)])
    const bubbles = wrapper.findAll('[data-testid="tg-post-tile"]')
    expect(bubbles).toHaveLength(2)
    expect(bubbles[0].text()).toContain('Post 1')
    expect(bubbles[1].text()).toContain('Post 2')
  })

  it('single-post payload renders the hero with zero bubbles', () => {
    const wrapper = mountCard([post(0)])
    expect(wrapper.find('[data-testid="tg-hero-post"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="tg-post-tile"]')).toHaveLength(0)
  })

  it('bubble posts without a link render as plain text (no anchor)', () => {
    const wrapper = mountCard([post(0), post(1, { link: undefined })])
    const bubble = wrapper.findAll('[data-testid="tg-post-tile"]')[0]
    expect(bubble.find('a').exists()).toBe(false)
    expect(bubble.text()).toContain('Post 1')
  })

  it('renders the subscribe CTA to the channel', () => {
    const wrapper = mountCard([post(0)])
    const sub = wrapper
      .findAll('a')
      .find((a) => a.attributes('href') === 'https://t.me/anime_enigma')
    expect(sub).toBeDefined()
    expect(sub!.text()).toContain('spotlight.telegramNews.subscribeCta')
    expect(sub!.attributes('rel')).toBe('noopener noreferrer')
  })

  it('renders the @anime_enigma channel attribution', () => {
    const wrapper = mountCard([post(0)])
    expect(wrapper.text()).toContain('@anime_enigma')
  })
})
