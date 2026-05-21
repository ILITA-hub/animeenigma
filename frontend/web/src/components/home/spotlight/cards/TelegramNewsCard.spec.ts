/**
 * Workstream hero-spotlight — Phase 3 (dynamic-cards-migration) Plan 03-05 / Task 2.
 *
 * Vitest spec for TelegramNewsCard.vue.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import TelegramNewsCard from './TelegramNewsCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(TelegramNewsCard, {
    props: props as unknown as InstanceType<typeof TelegramNewsCard>['$props'],
  })
}

const post = (i: number, overrides: Record<string, unknown> = {}) => ({
  title: `Title ${i}`,
  excerpt: `Excerpt body ${i} short.`,
  link: `https://t.me/animeenigma/${i}`,
  date: '2026-05-21',
  ...overrides,
})

describe('TelegramNewsCard', () => {
  it.each([1, 2, 3])('renders %i posts for %i entries', (n) => {
    const data = { posts: Array.from({ length: n }, (_, i) => post(i + 1)) }
    const wrapper = mountCard({ data })
    // Outer wrapper is itself an <article>; inner post articles carry the
    // bg-white/5 class. Count those specifically.
    const inner = wrapper.findAll('article.bg-white\\/5')
    expect(inner.length).toBe(n)
  })

  it('renders external anchor when post.link is present', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const anchors = wrapper.findAll('a')
    expect(anchors.length).toBeGreaterThanOrEqual(1)
    expect(anchors[0].attributes('href')).toBe('https://t.me/animeenigma/1')
    expect(anchors[0].attributes('target')).toBe('_blank')
    // Reverse-tabnabbing protection (T-03-18).
    expect(anchors[0].attributes('rel')).toBe('noopener noreferrer')
  })

  it('omits anchor when post.link is absent', () => {
    const data = { posts: [post(1, { link: undefined })] }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('a').length).toBe(0)
  })

  it('excerpt paragraph carries line-clamp-2 class', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    expect(wrapper.html()).toContain('line-clamp-2')
  })

  it('renders date when present', () => {
    const data = { posts: [post(1, { date: '2026-05-21' })] }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('2026-05-21')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
