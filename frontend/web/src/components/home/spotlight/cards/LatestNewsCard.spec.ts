/**
 * Workstream hero-spotlight — v4 E-1 lock (2026-06-11): terminal
 * changelog. Contract:
 *
 *   1. Single-root <article> (Transition out-in safety).
 *   2. Terminal panel: `$ animeenigma --updates` prompt + mono entries.
 *   3. Type → colored prefix: feature→[FEAT] cyan, fix→[FIX] success,
 *      perf→[PERF] warning, unknown/missing→[INFO] muted.
 *   4. Max 3 entries; messages clamp to 2 lines via .news-msg.
 *   5. Compact dd.MM gutter date (NOT the raw ISO string).
 *   6. «Читать всё» link-variant CTA scrolls to #changelog.
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

import LatestNewsCard from './LatestNewsCard.vue'

function isoDaysAgo(daysAgo: number): string {
  return new Date(Date.now() - daysAgo * 86_400_000).toISOString().slice(0, 10)
}

const baseData = {
  entries: [
    { date: isoDaysAgo(0), type: 'feature', message: 'Карусель пересобрана' },
    { date: isoDaysAgo(1), type: 'fix', message: 'Баннер починен' },
    { date: isoDaysAgo(2), type: 'perf', message: 'Картинки лениво' },
  ],
}

function mountCard(props: Record<string, unknown>) {
  return mount(LatestNewsCard, {
    props: props as unknown as InstanceType<typeof LatestNewsCard>['$props'],
  })
}

describe('LatestNewsCard — terminal (v4 E-1)', () => {
  it('renders a single root <article> element', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.element.tagName).toBe('ARTICLE')
  })

  it('renders the terminal panel with the prompt line and blink cursor', () => {
    const wrapper = mountCard({ data: baseData })
    const term = wrapper.find('[data-testid="terminal"]')
    expect(term.exists()).toBe(true)
    expect(term.text()).toContain('$ animeenigma --updates')
    expect(term.find('.cursor-blink').exists()).toBe(true)
    expect(term.classes().join(' ')).toContain('font-mono')
  })

  it('renders at most 3 terminal lines with the 2-line clamp class', () => {
    const five = {
      entries: Array.from({ length: 5 }, (_, i) => ({
        date: isoDaysAgo(i),
        type: 'feature',
        message: `entry ${i}`,
      })),
    }
    const wrapper = mountCard({ data: five })
    const lines = wrapper.findAll('[data-testid="terminal-line"]')
    expect(lines).toHaveLength(3)
    expect(lines[0].classes()).toContain('news-msg')
  })

  it('feature → [FEAT] cyan prefix', () => {
    const wrapper = mountCard({ data: baseData })
    const line = wrapper.findAll('[data-testid="terminal-line"]')[0]
    expect(line.text()).toContain('[FEAT]')
    expect(line.html()).toContain('text-cyan-400')
  })

  it('fix → [FIX] success prefix; perf → [PERF] warning prefix', () => {
    const wrapper = mountCard({ data: baseData })
    const lines = wrapper.findAll('[data-testid="terminal-line"]')
    expect(lines[1].text()).toContain('[FIX]')
    expect(lines[1].html()).toContain('text-success')
    expect(lines[2].text()).toContain('[PERF]')
    expect(lines[2].html()).toContain('text-warning')
  })

  it('unknown/missing type → muted [INFO] prefix', () => {
    const wrapper = mountCard({
      data: { entries: [{ date: isoDaysAgo(0), type: 'improvement', message: 'x' }] },
    })
    const line = wrapper.findAll('[data-testid="terminal-line"]')[0]
    expect(line.text()).toContain('[INFO]')
    expect(line.html()).toContain('text-muted-foreground')
  })

  it('renders a compact gutter date, not the raw ISO string', () => {
    const wrapper = mountCard({ data: baseData })
    const line = wrapper.findAll('[data-testid="terminal-line"]')[0]
    expect(line.text()).not.toContain(baseData.entries[0].date)
  })

  it('renders the readMore CTA', () => {
    const wrapper = mountCard({ data: baseData })
    expect(wrapper.text()).toContain('spotlight.latestNews.readMore')
  })
})
