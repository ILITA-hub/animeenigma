import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'

const row = (over: Partial<ProviderRow>): ProviderRow => ({
  def: { id: 'allanime', name: 'AllAnime', hue: '#00d4ff', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true },
  state: 'active', ...over,
} as ProviderRow)

describe('ProviderChip', () => {
  it('renders the provider name', () => {
    expect(mount(ProviderChip, { props: { row: row({}) } }).text()).toContain('AllAnime')
  })
  it('emits select when active and clicked', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'active' }) } })
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })
  it('is disabled and does NOT emit when state is disabled', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'disabled', reason: 'Cloudflare challenge' }) } })
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeFalsy()
    expect(w.find('button').attributes('disabled')).toBeDefined()
  })
  it('exposes the reason as a title/tooltip for non-active states', () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'wip', reason: 'We are working on our own hosting' }) } })
    expect(w.html()).toContain('We are working on our own hosting')
  })
  it('marks the active selection', () => {
    const w = mount(ProviderChip, { props: { row: row({}), selected: true } })
    expect(w.classes().join(' ')).toMatch(/is-active|is-selected/)
  })

  // --- capability label row ---
  const capMountOpts = { global: { mocks: { $t: (k: string) => k } } }
  const cap: ProviderCap = {
    provider: 'allanime', display_name: 'AllAnime', enabled: true, health: 'up', rank: 90,
    variants: [
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
      { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ],
  }

  it('renders category + quality tags when cap is present', () => {
    const w = mount(ProviderChip, { props: { row: row({}), cap }, ...capMountOpts })
    expect(w.findAll('[data-test="cap-cat"]').length).toBe(2)
    expect(w.find('[data-test="cap-quality"]').text()).toContain('1080p')
  })

  it('renders no label row without cap', () => {
    const w = mount(ProviderChip, { props: { row: row({}) }, ...capMountOpts })
    expect(w.find('[data-test="cap-cat"]').exists()).toBe(false)
  })

  it('shows the best pill when best=true', () => {
    const w = mount(ProviderChip, { props: { row: row({}), cap, best: true }, ...capMountOpts })
    expect(w.find('[data-test="cap-best"]').exists()).toBe(true)
  })

  // --- degraded (AUTO-484) ---
  it('renders the DEGRADED pill for a degraded row', () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'degraded', reason: 'ad wall' }) }, ...capMountOpts })
    expect(w.find('[data-test="cap-degraded"]').exists()).toBe(true)
  })
  it('degraded is NOT selectable without hacker mode', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'degraded' }) }, ...capMountOpts })
    expect(w.find('button').attributes('disabled')).toBeDefined()
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeFalsy()
  })
  it('degraded IS selectable in hacker mode', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'degraded' }), hackerMode: true }, ...capMountOpts })
    expect(w.find('button').attributes('disabled')).toBeUndefined()
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })

  // --- hacker-mode blurb (provider descriptions) ---
  const blurbRow = row({ def: { id: 'kodik', name: 'Kodik', hue: '#22d3ee', group: 'ru', audios: ['dub'], langs: ['ru'], content: ['common'], scraper: false, blurb: 'RU HLS — Russian dub & sub teams.' } })
  it('shows the provider blurb in hacker mode', () => {
    const w = mount(ProviderChip, { props: { row: blurbRow, hackerMode: true }, ...capMountOpts })
    expect(w.find('[data-test="provider-blurb"]').text()).toContain('RU HLS')
  })
  it('hides the provider blurb when not in hacker mode', () => {
    const w = mount(ProviderChip, { props: { row: blurbRow, hackerMode: false }, ...capMountOpts })
    expect(w.find('[data-test="provider-blurb"]').exists()).toBe(false)
  })
})
