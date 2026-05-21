/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-03 / Task 3.
 *
 * Vitest spec for LatestNewsCard.vue. Verifies:
 *   1. readMore router-link points to the verified changelog route (/)
 *      — see SUMMARY for the route-discovery note (no /changelog route
 *      exists; LastUpdates.vue is a home-page tab).
 *   2. data.entries.slice(0, 3) is respected — provide 5 entries, render 3
 *   3. Renders 1 entry when entries.length === 1
 *   4. Renders the entry's date + message text (matches the actual
 *      Phase 1 backend shape — see types/spotlight.ts ChangelogEntry)
 *   5. line-clamp-2 / line-clamp-3 utilities applied
 *   6. Only font-medium / font-semibold weights
 *   7. No hardcoded English — all labels via t()
 *
 * Deviation from plan template markup: Plan 02-03 §Action shows a template
 * that reads entry.title + entry.summary, but the type from Plan 02-01
 * defines ChangelogEntry as { date, type?, message }. The runtime payload
 * has no title/summary fields. Per Plan 02-01's own note ("Card components
 * must consume `message` for the body"), this spec asserts against the
 * `message` field. See SUMMARY.md for the documented deviation (Rule 1).
 */

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
  }),
}))

import LatestNewsCard from './LatestNewsCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(LatestNewsCard, {
    // Card components have a typed `data` prop; vue-tsc requires a concrete
    // shape rather than `Record<string, unknown>`. Cast at the boundary so
    // the helper stays generic across the three card specs.
    props: props as unknown as InstanceType<typeof LatestNewsCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const mock5 = {
  entries: [
    { date: '2026-05-21', type: 'feature', message: 'Phase 1 backend ships — spotlight aggregator delivered.' },
    { date: '2026-05-20', type: 'feature', message: 'Notifications service live — bell + dropdown GA.' },
    { date: '2026-05-18', type: 'fix', message: 'AnimePahe revival — stealth Chromium sidecar works.' },
    { date: '2026-05-15', type: 'improvement', message: 'Older entry 4.' },
    { date: '2026-05-10', type: 'feature', message: 'Older entry 5.' },
  ],
}

describe('LatestNewsCard', () => {
  it('renders the readMore link with a verified router target', () => {
    const wrapper = mountCard({ data: mock5 })
    // Find all RouterLinkStub instances. The readMore link is the only one
    // in the header — entry <li>s are NOT router-links in Phase 2 (the whole
    // entry is read-only text; clicking the readMore link is the affordance).
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.length).toBeGreaterThanOrEqual(1)
    // The header readMore link is the first router-link in DOM order.
    const readMore = links[0]
    // Phase 2 route-discovery: no /changelog route exists; LastUpdates is a
    // home-page tab. Plan 02-03 acceptance criterion explicitly defers the
    // exact path to executor verification. The link MUST go somewhere valid;
    // for Phase 2 we point at "/" so the changelog tab is in view.
    const to = readMore.props('to')
    expect(typeof to).toBe('string')
    expect(to).toBe('/')
  })

  it('renders up to 3 entries (caps at slice(0, 3))', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.findAll('li').length).toBe(3)
  })

  it('renders 1 entry when entries.length === 1', () => {
    const wrapper = mountCard({ data: { entries: [mock5.entries[0]] } })
    expect(wrapper.findAll('li').length).toBe(1)
  })

  it('renders the entry date and message text', () => {
    const wrapper = mountCard({ data: { entries: [mock5.entries[0]] } })
    const text = wrapper.text()
    expect(text).toContain('2026-05-21')
    expect(text).toContain('Phase 1 backend ships')
  })

  it('applies line-clamp utilities (line-clamp-2 on title, line-clamp-3 on summary)', () => {
    const wrapper = mountCard({ data: mock5 })
    const html = wrapper.html()
    expect(html).toContain('line-clamp-2')
    expect(html).toContain('line-clamp-3')
  })

  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: mock5 })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses tablet padding p-4 (never p-5)', () => {
    const wrapper = mountCard({ data: mock5 })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
  })

  it('has no hardcoded English text — all labels via t()', () => {
    const wrapper = mountCard({ data: mock5 })
    const text = wrapper.text()
    expect(text).toContain('spotlight.latestNews.title')
    expect(text).toContain('spotlight.latestNews.readMore')
    // Raw English copy should NOT leak.
    expect(text).not.toMatch(/What's new/)
    expect(text).not.toMatch(/Read full changelog/)
  })

  it('uses md:grid-cols-3 layout (desktop 3-col grid)', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.html()).toContain('md:grid-cols-3')
  })
})
