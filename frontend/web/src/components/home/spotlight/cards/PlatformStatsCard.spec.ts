import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// The DS-aligned card renders its kicker via useI18n — echo-key mock, same
// pattern as the sibling card specs.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'ru' },
  }),
}))

import PlatformStatsCard from './PlatformStatsCard.vue'
import type { PlatformStatsData } from '@/types/spotlight'

const base: PlatformStatsData = {
  hero: {
    working_ok: true,
    uptime_percent: 99.4,
    uptime_quip: 'ОЧЕНЬ МНОГО',
    service: 'catalog',
    ux_delta: '+5 (Tremendous)',
    cdi: '0.00 * 99',
    mvq: 'Dragon 99%/99%',
    tagline: 'Лучшая платформа. Поверьте.',
  },
  tiles: [
    { label: 'Запросов обработано', value: 48201, window: 'day', format: 'int' },
    { label: 'Данных отдано', value: 1610612736, window: 'week', format: 'bytes' },
  ],
}

const clone = (over: Partial<PlatformStatsData['hero']> = {}): PlatformStatsData => ({
  hero: { ...base.hero, ...over },
  tiles: base.tiles,
})

describe('PlatformStatsCard (joke)', () => {
  it('renders ДА when working_ok is true', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.text()).toContain('ДА')
    expect(w.text()).not.toContain('ТЕХНИЧЕСКИ ДА')
  })

  it('renders ТЕХНИЧЕСКИ ДА when working_ok is false', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone({ working_ok: false }) } })
    expect(w.text()).toContain('ТЕХНИЧЕСКИ ДА')
  })

  it('renders the uptime quip + percent, and omits percent when null', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.text()).toContain('ОЧЕНЬ МНОГО')
    expect(w.text()).toContain('99.4%')
    // mvq is overridden to a %-free value so the not.toContain('%') below
    // proves the *uptime percent* is omitted, not just that some '%' is gone
    // (base.hero.mvq is 'Dragon 99%/99%', which would otherwise contaminate it).
    const w2 = mount(PlatformStatsCard, {
      props: { data: clone({ uptime_percent: null, mvq: 'Dragon top/top' }) },
    })
    expect(w2.text()).toContain('ОЧЕНЬ МНОГО')
    expect(w2.text()).not.toContain('%')
  })

  it('renders the vibe row and tagline', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    const txt = w.text()
    expect(txt).toContain('catalog')
    expect(txt).toContain('+5 (Tremendous)')
    expect(txt).toContain('Dragon 99%/99%')
    expect(txt).toContain('Лучшая платформа. Поверьте.')
  })

  it('renders one tile per entry with Russian window badges', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.findAll('li')).toHaveLength(2)
    expect(w.text()).toContain('ЗА ДЕНЬ')
    expect(w.text()).toContain('ЗА НЕДЕЛЮ')
  })

  it('renders hero only when there are no tiles', () => {
    const w = mount(PlatformStatsCard, { props: { data: { hero: base.hero, tiles: [] } } })
    expect(w.findAll('li')).toHaveLength(0)
    expect(w.text()).toContain('ДА')
  })
})
