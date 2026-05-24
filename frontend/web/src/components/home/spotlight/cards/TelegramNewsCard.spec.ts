/**
 * Workstream hero-spotlight — v1.1-polish Phase 06 (HSB-V11-TG-04).
 *
 * Vitest spec for the branded TelegramNewsCard.vue. Replaces the Phase 03
 * spec which targeted the old plain `bg-white/5` post tiles — the v1.1
 * refactor swaps to `bg-black/30 backdrop-blur-sm` tiles riding on the
 * sky gradient-mesh backdrop.
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

describe('TelegramNewsCard (v1.1-polish)', () => {
  it('has a single <article> root element (Transition out-in safety)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    // Vue 3 <Transition mode="out-in"> silently wedges if a card has a top-
    // level v-if, multiple root elements, or leading template comments.
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it.each([1, 2, 3])('renders %i post tiles for %i entries', (n) => {
    const data = { posts: Array.from({ length: n }, (_, i) => post(i + 1)) }
    const wrapper = mountCard({ data })
    // Inner post tiles carry the bg-black/30 class; outer wrapper does not.
    const inner = wrapper.findAll('article.bg-black\\/30')
    expect(inner.length).toBe(n)
  })

  it('renders SpotlightBackdrop with gradient-mesh + sky accent', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    // SpotlightBackdrop emits a data-testid mesh element when in gradient-
    // mesh mode (asserted in SpotlightBackdrop.spec.ts).
    const mesh = wrapper.find('[data-testid="spotlight-backdrop-mesh"]')
    expect(mesh.exists()).toBe(true)
  })

  it('renders SpotlightIcon name="telegram" in the header with aria-label', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    // The icon SVG carries the forwarded class; SpotlightIcon root <svg>
    // ships aria-hidden="true" by default, but the caller also passes
    // aria-label="Telegram" which goes onto the root via $attrs (inheritAttrs
    // is false in SpotlightIcon so $attrs lands on the <svg>).
    const tgIcon = wrapper.find('svg[aria-label="Telegram"]')
    expect(tgIcon.exists()).toBe(true)
  })

  it('shows the @anime_enigma channel attribution', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('@anime_enigma')
  })

  it('renders thumbnail <img> when post.image_url is present', () => {
    const data = {
      posts: [
        post(1, { image_url: 'https://cdn4.telesco.pe/file/abcdef.jpg' }),
      ],
    }
    const wrapper = mountCard({ data })
    const imgs = wrapper.findAll('img')
    expect(imgs.length).toBe(1)
    expect(imgs[0].attributes('src')).toBe(
      'https://cdn4.telesco.pe/file/abcdef.jpg',
    )
    // Lazy-load every thumbnail — these are below-the-fold from the
    // visitor's viewport on the home page.
    expect(imgs[0].attributes('loading')).toBe('lazy')
    // Alt fallback to post.title when present.
    expect(imgs[0].attributes('alt')).toBe('Title 1')
  })

  it('omits thumbnail when post.image_url is absent (layout collapses)', () => {
    const data = { posts: [post(1, { image_url: undefined })] }
    const wrapper = mountCard({ data })
    // No <img> in the entire card when image_url is not supplied.
    expect(wrapper.findAll('img').length).toBe(0)
  })

  it('falls back to empty alt when image_url present but title absent', () => {
    const data = {
      posts: [
        {
          excerpt: 'excerpt only',
          image_url: 'https://cdn4.telesco.pe/file/xx.jpg',
        },
      ],
    }
    const wrapper = mountCard({ data })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('alt')).toBe('')
  })

  it('renders external anchor with rel="noopener noreferrer" (T-03-18)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const anchors = wrapper.findAll('a')
    expect(anchors.length).toBeGreaterThanOrEqual(1)
    expect(anchors[0].attributes('href')).toBe('https://t.me/animeenigma/1')
    expect(anchors[0].attributes('target')).toBe('_blank')
    expect(anchors[0].attributes('rel')).toBe('noopener noreferrer')
  })

  it('anchor wears .cta-text + data-accent="sky" (Phase 01 CTA system)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const anchor = wrapper.find('a')
    expect(anchor.classes()).toContain('cta-text')
    expect(anchor.attributes('data-accent')).toBe('sky')
  })

  it('omits anchor entirely when post.link is absent', () => {
    const data = { posts: [post(1, { link: undefined })] }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('a').length).toBe(0)
  })

  it('excerpt paragraph carries line-clamp-3 (longer excerpts than v1.0)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    expect(wrapper.html()).toContain('line-clamp-3')
  })

  it('renders date when present', () => {
    const data = { posts: [post(1, { date: '2026-05-21' })] }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('2026-05-21')
  })

  it('uses only font-medium and font-semibold typography weights (UI-SPEC)', () => {
    const data = {
      posts: [
        post(1, {
          image_url: 'https://cdn4.telesco.pe/file/x.jpg',
        }),
      ],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses responsive padding p-4 / md:p-6 / lg:p-8 (UI-SPEC)', () => {
    const data = { posts: [post(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    // Banned tablet padding.
    expect(html).not.toMatch(/\bp-5\b/)
    // Required mobile/tablet/desktop scale.
    expect(html).toMatch(/\bp-4\b/)
    expect(html).toMatch(/md:p-6/)
    expect(html).toMatch(/lg:p-8/)
  })
})
