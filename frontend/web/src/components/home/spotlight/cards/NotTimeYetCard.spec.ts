/**
 * Workstream hero-spotlight — v1.1-polish Phase 09 (HSB-V11-NT-01..04).
 *
 * Vitest spec for the NotTimeYetCard amber/clock refactor. Phase 3 spec
 * verified the cyan list-passthrough layout; Phase 09 replaces it with:
 *
 *   1. Single-root <article> + SpotlightBackdrop (poster-blur, amber).
 *   2. Amber secondary gradient overlay (from-amber-500/30).
 *   3. Clock-icon header (SpotlightIcon name="clock").
 *   4. Status pill — yellow for planned, slate for postponed.
 *   5. Relative "Added X ago" line via formatAgo when added_at present.
 *   6. Direct-to-watch CTA: href ends in /watch (not the detail page).
 *
 * The `t` mock echoes the key (+ JSON params) so assertions don't need an
 * i18n bundle — same pattern as LatestNewsCard.spec.ts.
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

import NotTimeYetCard from './NotTimeYetCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(NotTimeYetCard, {
    props: props as unknown as InstanceType<typeof NotTimeYetCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

// ISO timestamp `daysAgo` calendar days in the past so the relative-date
// assertion stays deterministic regardless of run time.
function isoDaysAgo(daysAgo: number): string {
  return new Date(Date.now() - daysAgo * 86_400_000).toISOString()
}

const baseData = {
  status: 'planned' as const,
  added_at: isoDaysAgo(14),
  anime: {
    id: 'a-1',
    name: 'Frieren',
    name_ru: 'Фрирен',
    poster_url: '/poster.jpg',
    episodes_count: 28,
  },
}

describe('NotTimeYetCard — root + backdrop', () => {
  it('renders a single root <article> element', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('renders the violet secondary gradient overlay (brand triad)', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.html()).toContain('from-brand-violet/25')
  })

  it('renders a poster-blur backdrop driven by the poster url', () => {
    const wrapper = mountCard({ data: baseData })
    // SpotlightBackdrop renders the blurred <img> for the poster-blur
    // variant when a posterUrl is supplied.
    const imgs = wrapper.findAll('img')
    expect(imgs.some((i) => i.attributes('src') === '/poster.jpg')).toBe(true)
  })
})

describe('NotTimeYetCard — header + status pill', () => {
  it('renders the clock SpotlightIcon in the header', () => {
    const wrapper = mountCard({ data: baseData })
    // SpotlightIcon renders an <svg>; the clock variant is selected via
    // its `name` prop. We assert the icon component is present alongside
    // the promoted title key.
    expect(wrapper.findComponent({ name: 'SpotlightIcon' }).exists()).toBe(true)
    expect(wrapper.text()).toContain('spotlight.notTimeYet.title')
  })

  it('shows the planned status label + warning pill class', () => {
    const wrapper = mountCard({ data: { ...baseData, status: 'planned' } })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.statusPlanned')
    expect(wrapper.text()).not.toContain('spotlight.notTimeYet.statusPostponed')
    // DS alignment: status renders as an overlay Badge (dark glass) with a
    // warning accent TEXT class for planned entries.
    expect(wrapper.html()).toContain('text-warning')
    expect(wrapper.html()).toContain('bg-black/[0.62]')
  })

  it('shows the postponed status label + muted pill class', () => {
    const wrapper = mountCard({ data: { ...baseData, status: 'postponed' } })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.statusPostponed')
    expect(wrapper.text()).not.toContain('spotlight.notTimeYet.statusPlanned')
    expect(wrapper.html()).toContain('text-muted-foreground')
    expect(wrapper.html()).toContain('bg-black/[0.62]')
  })
})

describe('NotTimeYetCard — addedAt + CTA', () => {
  it('renders the relative addedAt line when added_at is provided', () => {
    const wrapper = mountCard({ data: baseData })
    // The `t` mock echoes the addedAt key with its params; formatAgo turns
    // a 14-day-old timestamp into a "weeks ago" relative string.
    expect(wrapper.text()).toContain('spotlight.notTimeYet.addedAt')
    expect(wrapper.text()).toContain('weeks ago')
  })

  it('hides the addedAt line when added_at is absent', () => {
    const noDate = { ...baseData, added_at: null }
    const wrapper = mountCard({ data: noDate })
    expect(wrapper.text()).not.toContain('spotlight.notTimeYet.addedAt')
  })

  it('CTA links directly to the watch page (/anime/{id}/watch)', () => {
    const wrapper = mountCard({ data: baseData })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const cta = links.find((l) => l.props('to') === '/anime/a-1/watch')
    expect(cta).toBeDefined()
    expect((cta!.props('to') as string).endsWith('/watch')).toBe(true)
  })

  it('renders the watchCta label on the CTA', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.notTimeYet.watchCta')
  })
})

describe('NotTimeYetCard — style discipline', () => {
  it('renders the poster with anime.poster_url', () => {
    const wrapper = mountCard({ data: baseData })
    const img = wrapper.findAll('img').find((i) => i.attributes('loading') === 'lazy')
    expect(img).toBeDefined()
    expect(img!.attributes('src')).toBe('/poster.jpg')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses the p-4 md:p-6 lg:p-8 padding ladder (never p-5)', () => {
    const wrapper = mountCard({ data: baseData })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })
})
