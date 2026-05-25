/**
 * Workstream hero-spotlight — v1.1-polish Phase 07 (HSB-V11-LN-01..04).
 *
 * Vitest spec for the LatestNewsCard refactor. Phase 02 spec verified
 * the message-passthrough layout; Phase 07 layers on:
 *
 *   1. Single-root <article> + SpotlightBackdrop (gradient-mesh amber).
 *   2. Per-entry type icon (feat→sparkles, fix→wrench, perf→lightning).
 *   3. Per-entry type pill (cyan / green / amber accents from cardTokens).
 *   4. Relative date via Intl.RelativeTimeFormat (locale-aware).
 *   5. The Phase 02 sentence-splitter regex is removed from the source.
 *
 * The Phase 02 assertions that still apply (3-cap, single-entry render,
 * grid-cols-3, font-weight discipline) are preserved verbatim.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    // Echo the key back so assertions don't need an i18n bundle. Numeric
    // locale strings are echoed as a Ref<string>-lookalike so the SFC's
    // `(i18nLocale as { value?: unknown }).value` access path resolves
    // to a real locale code.
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import LatestNewsCard from './LatestNewsCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(LatestNewsCard, {
    props: props as unknown as InstanceType<typeof LatestNewsCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

// Compute the absolute path to the SFC source so the regex-absence test
// can read it without bundler interference. Resolved relative to this
// spec file so it survives a repo-relocate.
const __dirname = path.dirname(fileURLToPath(import.meta.url))
const LATEST_NEWS_VUE_PATH = path.join(__dirname, 'LatestNewsCard.vue')
const LATEST_NEWS_SOURCE = fs.readFileSync(LATEST_NEWS_VUE_PATH, 'utf8')

// Build an ISO date string `daysAgo` calendar days in the past so the
// relative-date assertion is deterministic regardless of run time.
function isoDaysAgo(daysAgo: number): string {
  const d = new Date()
  d.setUTCDate(d.getUTCDate() - daysAgo)
  return d.toISOString().slice(0, 10)
}

const mock5 = {
  entries: [
    { date: '2026-05-21', type: 'feat', message: 'Phase 1 backend ships — spotlight aggregator delivered.' },
    { date: '2026-05-20', type: 'feature', message: 'Notifications service live — bell + dropdown GA.' },
    { date: '2026-05-18', type: 'fix', message: 'AnimePahe revival — stealth Chromium sidecar works.' },
    { date: '2026-05-15', type: 'perf', message: 'Older entry 4.' },
    { date: '2026-05-10', type: 'feat', message: 'Older entry 5.' },
  ],
}

describe('LatestNewsCard — root + backdrop', () => {
  it('renders a single root <article> element', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('wraps the foreground in a SpotlightBackdrop with gradient-mesh + amber accent', () => {
    const wrapper = mountCard({ data: mock5 })
    // Two markers: the backdrop component's mesh testid + the amber
    // tinted radial-gradient class string (proves accent prop wired).
    expect(wrapper.find('[data-testid="spotlight-backdrop-mesh"]').exists()).toBe(true)
    expect(wrapper.html()).toMatch(/rgba\(251,191,36/)
  })
})

describe('LatestNewsCard — header CTA', () => {
  it('renders the readMore anchor pointing to #changelog', () => {
    const wrapper = mountCard({ data: mock5 })
    const anchor = wrapper.find('a[href="#changelog"]')
    expect(anchor.exists()).toBe(true)
    expect(anchor.text()).toContain('spotlight.latestNews.readMore')
    expect(wrapper.findAllComponents(RouterLinkStub).length).toBe(0)
  })

  it('renders the header sparkles icon', () => {
    const wrapper = mountCard({ data: mock5 })
    // The header icon is rendered before any entries — pick the first
    // SVG and assert its sparkles-distinctive path is in the markup.
    const html = wrapper.html()
    // sparkles icon has 8 cardinal-direction tick marks
    expect(html).toMatch(/M12 3v3M12 18v3/)
  })
})

describe('LatestNewsCard — entries rendering', () => {
  it('caps entries at 3 (slice(0, 3))', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.findAll('li').length).toBe(3)
  })

  it('renders 1 entry when entries.length === 1', () => {
    const wrapper = mountCard({ data: { entries: [mock5.entries[0]] } })
    expect(wrapper.findAll('li').length).toBe(1)
  })

  it('uses md:grid-cols-3 layout (desktop 3-col grid)', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.html()).toContain('md:grid-cols-3')
  })
})

describe('LatestNewsCard — per-entry type icon + pill', () => {
  it('feat entry renders sparkles icon + cyan-accented pill', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'feat', message: 'New feature shipped.' }] },
    })
    const html = wrapper.html()
    // sparkles signature path
    expect(html).toMatch(/M12 3v3M12 18v3/)
    // cyan pill accent
    expect(html).toContain('bg-cyan-500/20')
    expect(html).toContain('text-cyan-200')
    // pill text is the i18n key (mock t() echoes)
    expect(wrapper.text()).toContain('spotlight.latestNews.typeFeat')
  })

  it('fix entry renders wrench icon + green-accented pill', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'fix', message: 'Bug squashed.' }] },
    })
    const html = wrapper.html()
    // wrench signature path
    expect(html).toMatch(/M14 7a4 4 0 1 1 3\.6 3\.96L9 19\.5/)
    expect(html).toContain('bg-green-500/20')
    expect(html).toContain('text-green-200')
    expect(wrapper.text()).toContain('spotlight.latestNews.typeFix')
  })

  it('perf entry renders lightning icon + amber-accented pill', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'perf', message: 'Faster!' }] },
    })
    const html = wrapper.html()
    // lightning signature path
    expect(html).toMatch(/M13 2 3 14h7l-1 8/)
    expect(html).toContain('bg-amber-500/20')
    expect(html).toContain('text-amber-200')
    expect(wrapper.text()).toContain('spotlight.latestNews.typePerf')
  })

  it('falls back gracefully for unknown type (no pill, gray icon)', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'wat', message: 'Mystery entry.' }] },
    })
    const html = wrapper.html()
    expect(html).toContain('text-gray-300')
    // No type pill labels should render
    expect(wrapper.text()).not.toContain('spotlight.latestNews.typeFeat')
    expect(wrapper.text()).not.toContain('spotlight.latestNews.typeFix')
    expect(wrapper.text()).not.toContain('spotlight.latestNews.typePerf')
  })

  it('handles entries with no type field (no pill, sparkles fallback icon)', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', message: 'No type at all.' }] },
    })
    const html = wrapper.html()
    expect(html).toMatch(/M12 3v3M12 18v3/) // sparkles fallback
    expect(html).toContain('text-gray-300')
  })

  it('covers long-form "feature"/"improvement" synonyms emitted by changelog.json today', () => {
    const wrapper = mountCard({
      data: {
        entries: [
          { date: '2026-05-20', type: 'feature', message: 'Long-form feature.' },
          { date: '2026-05-19', type: 'improvement', message: 'Long-form improvement.' },
        ],
      },
    })
    // Both pills must render — without synonym coverage they'd silently
    // fall through to the no-pill branch.
    expect(wrapper.text()).toContain('spotlight.latestNews.typeFeat')
    expect(wrapper.text()).toContain('spotlight.latestNews.typePerf')
  })
})

describe('LatestNewsCard — relative date formatting', () => {
  it('renders a relative date (NOT the raw ISO string) for a recent entry', () => {
    const twoDaysAgo = isoDaysAgo(2)
    const wrapper = mountCard({
      data: { entries: [{ date: twoDaysAgo, type: 'feat', message: 'Recent' }] },
    })
    const text = wrapper.text()
    // The raw ISO date should be absent — Intl.RelativeTimeFormat output
    // never matches YYYY-MM-DD for entries inside the relative window.
    expect(text).not.toContain(twoDaysAgo)
    // And the locale-en relative output for -2 days starts with "2".
    expect(text).toMatch(/2 days ago|day ago|days ago/i)
  })

  it('falls back to the raw ISO string when the date is unparseable', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: 'not-a-date', type: 'feat', message: 'Garbage in.' }] },
    })
    expect(wrapper.text()).toContain('not-a-date')
  })
})

describe('LatestNewsCard — message truncation', () => {
  it('passes through messages <= 60 chars unchanged', () => {
    const short = 'Phase 1 backend ships — spotlight aggregator delivered.'
    expect(short.length).toBeLessThanOrEqual(60)
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'feat', message: short }] },
    })
    expect(wrapper.text()).toContain(short)
  })

  it('truncates messages > 60 chars with an ellipsis', () => {
    const long = 'A very long message that absolutely positively definitely exceeds the sixty character cutoff threshold.'
    expect(long.length).toBeGreaterThan(60)
    const wrapper = mountCard({
      data: { entries: [{ date: '2026-05-21', type: 'feat', message: long }] },
    })
    const text = wrapper.text()
    expect(text).toContain('…')
    expect(text).not.toContain(long) // full message must not appear
  })
})

describe('LatestNewsCard — typography + a11y discipline', () => {
  it('uses only font-medium and font-semibold typography weights', () => {
    const wrapper = mountCard({ data: mock5 })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses the Phase 07 padding scale (p-4 / md:p-6 / lg:p-8)', () => {
    const wrapper = mountCard({ data: mock5 })
    const html = wrapper.html()
    expect(html).toContain('p-4')
    expect(html).toContain('md:p-6')
    expect(html).toContain('lg:p-8')
    // Never use the disallowed p-5 token (Phase 02 contract).
    expect(html).not.toMatch(/\bp-5\b/)
  })

  it('applies line-clamp-3 on the message title (max 3 lines)', () => {
    const wrapper = mountCard({ data: mock5 })
    expect(wrapper.html()).toContain('line-clamp-3')
  })

  it('has no hardcoded English title — uses t()', () => {
    const wrapper = mountCard({ data: mock5 })
    const text = wrapper.text()
    expect(text).toContain('spotlight.latestNews.title')
    expect(text).toContain('spotlight.latestNews.readMore')
    expect(text).not.toMatch(/What's new/)
    expect(text).not.toMatch(/Read full changelog/)
  })
})

describe('LatestNewsCard — source-level invariants (Phase 07)', () => {
  it('source no longer contains the Phase 02 sentence-splitter regex', () => {
    // The exact regex literal lived inside `splitMessage()`. We assert
    // both the function name AND a distinctive fragment of the regex
    // are absent so a partial revert is caught.
    expect(LATEST_NEWS_SOURCE).not.toContain('splitMessage')
    expect(LATEST_NEWS_SOURCE).not.toMatch(/\^\(\.\+\?\[\.!\?/)
  })

  it('source no longer contains the deprecated entryBody helper', () => {
    expect(LATEST_NEWS_SOURCE).not.toContain('function entryBody')
  })
})
