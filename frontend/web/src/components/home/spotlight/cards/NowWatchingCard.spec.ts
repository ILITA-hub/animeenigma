/**
 * Workstream hero-spotlight — v1.1-polish Phase 05 (HSB-V11-NW-01..04).
 *
 * Vitest spec for the refactored NowWatchingCard.vue. Verifies (against
 * the new social-identity layout):
 *
 *  1. Single root <article> with no top-level siblings (Phase 04 footgun:
 *     <transition mode="out-in"> wedges if root is a comment node).
 *  2. SpotlightBackdrop rendered with variant="gradient-mesh" accent="green".
 *  3. Header SpotlightIcon name="pulse" carries animate-pulse class.
 *  4. Each row has an avatar circle whose text content starts with
 *     `username[0].toUpperCase()`.
 *  5. avatarBgClass deterministic — same username → same class across
 *     two independent mounts.
 *  6. avatarBgClass distribution — the returned class always belongs
 *     to the 8-color palette.
 *  7. Each row renders a pulsing green LIVE dot
 *     (`bg-green-400 animate-pulse` span next to the avatar) — not a
 *     right-edge "LIVE" text label.
 *  8. Poster <img> has class `w-14` (56px) and inline `height: 84px`.
 *  9. Up to 3 rows render (data.sessions.slice(0, 3)).
 * 10. Typography contract: only font-medium / font-semibold weights.
 * 11. Padding p-4 / md:p-6 / lg:p-8 (no p-5).
 * 12. Each row links to /anime/{anime_id}.
 * 13. Liveness "LIVE" text preserved as sr-only for a11y + e2e
 *     (spotlight-full text=LIVE check uses toBeAttached, not visible).
 */

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

import NowWatchingCard from './NowWatchingCard.vue'

// 8-color avatar palette — kept in sync with the SFC's const PALETTE.
// Used by the "class belongs to palette" assertions below.
const PALETTE = [
  'bg-destructive',
  'bg-orange-500',
  'bg-warning',
  'bg-success',
  'bg-cyan-500',
  'bg-info',
  'bg-brand-violet',
  'bg-pink-500',
] as const

function mountCard(props: Record<string, unknown>) {
  return mount(NowWatchingCard, {
    props: props as unknown as InstanceType<typeof NowWatchingCard>['$props'],
    global: {
      stubs: {
        'router-link': RouterLinkStub,
        // Stub the two design-system components so we can assert their
        // presence + forwarded props without depending on their
        // decorative SVG/gradient internals.
        SpotlightBackdrop: {
          name: 'SpotlightBackdrop',
          props: ['variant', 'accent', 'posterUrl'],
          template:
            '<div data-testid="backdrop" :data-variant="variant" :data-accent="accent"></div>',
        },
        SpotlightIcon: {
          name: 'SpotlightIcon',
          props: ['name'],
          // Forward `class` so the spec can assert animate-pulse on the
          // header icon (SpotlightIcon's real component uses
          // inheritAttrs:false + manual class forwarding).
          inheritAttrs: false,
          template:
            '<span data-testid="icon" :data-name="name" :class="$attrs.class"></span>',
        },
      },
    },
  })
}

const session = (i: number, overrides: Record<string, unknown> = {}) => ({
  username: `u${i}`,
  public_id: `pid-${i}`,
  anime_id: `anime-${i}`,
  anime_name: `AnimeName${i}`,
  anime_name_ru: `Аниме${i}`,
  poster_url: `/poster-${i}.jpg`,
  episode_number: i + 4,
  updated_at: '2026-05-21T10:00:00Z',
  ...overrides,
})

describe('NowWatchingCard (v1.1-polish social-identity layout)', () => {
  // ── Root element + transition-mode safety ────────────────────────────

  it('root element is a single <article> with no sibling roots', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    expect(wrapper.element.tagName).toBe('ARTICLE')
    // Vue's mount root must be exactly one element to keep
    // <transition mode="out-in"> from wedging on a comment-node root.
    const parent = (wrapper.element as Element).parentElement
    expect(parent?.children.length).toBe(1)
  })

  // ── Backdrop ─────────────────────────────────────────────────────────

  it('renders SpotlightBackdrop with variant="gradient-mesh" accent="green"', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const backdrop = wrapper.find('[data-testid="backdrop"]')
    expect(backdrop.exists()).toBe(true)
    expect(backdrop.attributes('data-variant')).toBe('gradient-mesh')
    expect(backdrop.attributes('data-accent')).toBe('green')
  })

  // ── Header pulse icon ────────────────────────────────────────────────

  it('header SpotlightIcon name="pulse" carries animate-pulse', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const icon = wrapper.find('[data-testid="icon"]')
    expect(icon.exists()).toBe(true)
    expect(icon.attributes('data-name')).toBe('pulse')
    expect(icon.classes()).toContain('animate-pulse')
  })

  // ── Row count ────────────────────────────────────────────────────────

  it.each([1, 2, 3])('renders %i rows for %i sessions', (n) => {
    const data = {
      sessions: Array.from({ length: n }, (_, i) => session(i + 1)),
    }
    const wrapper = mountCard({ data })
    const rows = wrapper.findAllComponents(RouterLinkStub)
    expect(rows.length).toBe(n)
  })

  it('caps row count at 3 even with 5 sessions', () => {
    const data = {
      sessions: Array.from({ length: 5 }, (_, i) => session(i + 1)),
    }
    const wrapper = mountCard({ data })
    expect(wrapper.findAllComponents(RouterLinkStub).length).toBe(3)
  })

  // ── Avatar initial + deterministic color ─────────────────────────────

  it('avatar circle text content starts with username[0].toUpperCase()', () => {
    const data = {
      sessions: [
        session(1, { username: 'alice' }),
        session(2, { username: 'bob' }),
        session(3, { username: 'cheryl' }),
      ],
    }
    const wrapper = mountCard({ data })
    const rows = wrapper.findAllComponents(RouterLinkStub)
    // First child of each row anchor is the avatar circle <div>.
    expect(
      rows[0].element.children[0].textContent?.trim().charAt(0),
    ).toBe('A')
    expect(
      rows[1].element.children[0].textContent?.trim().charAt(0),
    ).toBe('B')
    expect(
      rows[2].element.children[0].textContent?.trim().charAt(0),
    ).toBe('C')
  })

  it('avatar background color is deterministic — same username → same class across mounts', () => {
    const data1 = { sessions: [session(1, { username: 'alice' })] }
    const data2 = { sessions: [session(1, { username: 'alice' })] }

    const w1 = mountCard({ data: data1 })
    const w2 = mountCard({ data: data2 })

    // Pull the bg-* class string off the first row's avatar div.
    const classes1 = w1
      .findComponent(RouterLinkStub)
      .element.children[0].classList
    const classes2 = w2
      .findComponent(RouterLinkStub)
      .element.children[0].classList

    const palette1 = PALETTE.find((c) => classes1.contains(c))
    const palette2 = PALETTE.find((c) => classes2.contains(c))

    expect(palette1).toBeDefined()
    expect(palette2).toBeDefined()
    expect(palette1).toBe(palette2)
  })

  it('avatar background class always belongs to the 8-color palette', () => {
    const data = {
      sessions: [
        session(1, { username: 'alice' }),
        session(2, { username: 'bob' }),
        session(3, { username: 'charlotte' }),
      ],
    }
    const wrapper = mountCard({ data })
    const rows = wrapper.findAllComponents(RouterLinkStub)

    for (const row of rows) {
      const classList = row.element.children[0].classList
      const hit = PALETTE.find((c) => classList.contains(c))
      expect(
        hit,
        `expected a palette class on ${Array.from(classList).join(' ')}`,
      ).toBeDefined()
    }
  })

  // ── Pulsing LIVE dot (next to avatar, not right-edge text) ───────────

  it('each row renders a pulsing green LIVE dot adjacent to the avatar', () => {
    const data = { sessions: [session(1), session(2), session(3)] }
    const wrapper = mountCard({ data })
    // The dot is a <span> with bg-success + animate-pulse INSIDE the
    // avatar circle. Three rows = three dots.
    const dots = wrapper.findAll('span.bg-success.animate-pulse')
    expect(dots.length).toBe(3)
  })

  it('does NOT render a right-edge "LIVE" text label (v1.0 behavior)', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    // The old behavior placed `t('spotlight.nowWatching.liveBadge')`
    // inside a `<span class="ml-auto text-xs ... text-green-400">`. The
    // new design moves the LIVE indicator into the avatar circle as an
    // sr-only label, so no `ml-auto ... text-green-400` text element
    // exists in the rendered HTML.
    const html = wrapper.html()
    expect(html).not.toMatch(/ml-auto[^"]*text-green-400/)
  })

  it('preserves "LIVE" text inside sr-only for a11y + spotlight-full e2e', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const srOnly = wrapper.find('span.sr-only')
    expect(srOnly.exists()).toBe(true)
    // Our t() mock echoes the key — assert the expected i18n key is used.
    expect(srOnly.text()).toBe('spotlight.nowWatching.liveBadge')
  })

  // ── Poster size ──────────────────────────────────────────────────────

  it('poster img is 56px wide via w-14 utility', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(img.classes()).toContain('w-14')
  })

  it('poster img is 84px tall via inline style', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    // Inline `style="height: 84px"` — assert via attribute substring so
    // we don't depend on Vue's exact style serialization.
    expect(img.attributes('style') || '').toMatch(/height:\s*84px/)
  })

  it('omits avatar img when poster_url missing', () => {
    const data = { sessions: [session(1, { poster_url: undefined })] }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('img').length).toBe(0)
  })

  // ── Linkage + content ────────────────────────────────────────────────

  it('links each row to /anime/{id}', () => {
    const data = { sessions: [session(1), session(2)] }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links[0].props('to')).toBe('/anime/anime-1')
    expect(links[1].props('to')).toBe('/anime/anime-2')
  })

  it('row text content includes username, localized anime name, and episode number', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const text = wrapper.text()
    expect(text).toContain('u1')
    expect(text).toContain('AnimeName1')
    expect(text).toContain('ep 5')
  })

  // ── Typography + padding contract ────────────────────────────────────

  it('uses only font-medium and font-semibold typography weights', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses p-4 / md:p-6 / lg:p-8 padding (never p-5)', () => {
    const data = { sessions: [session(1)] }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
    expect(html).toMatch(/\bmd:p-6\b/)
    expect(html).toMatch(/\blg:p-8\b/)
  })
})
