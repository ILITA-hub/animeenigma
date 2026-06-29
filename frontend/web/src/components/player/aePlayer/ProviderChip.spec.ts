import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'

const base: ProviderRow = {
  id: 'gogoanime', label: 'GogoAnime', group: 'en', state: 'active',
  selectable: true, hackerOnly: false, order: 85, audios: ['sub'],
}
const row = (over: Partial<ProviderRow> = {}): ProviderRow => ({ ...base, ...over })

const stub = { global: { mocks: { $t: (k: string) => k } } }

describe('ProviderChip', () => {
  it('renders the provider label', () => {
    expect(mount(ProviderChip, { props: { row: row() }, ...stub }).text()).toContain('GogoAnime')
  })

  it('emits select when active+selectable and clicked', async () => {
    const w = mount(ProviderChip, { props: { row: row() }, ...stub })
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })

  it('no_content is tinted and not selectable', async () => {
    const w = mount(ProviderChip, {
      props: { row: row({ state: 'no_content', selectable: false, reason: 'No episodes' }) },
      ...stub,
    })
    expect(w.find('button').attributes('disabled')).toBeDefined()
    expect(w.find('[data-test="cap-nocontent"]').exists()).toBe(true)
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeFalsy()
  })

  it('exposes reason as the tooltip title', () => {
    const w = mount(ProviderChip, {
      props: { row: row({ state: 'no_content', selectable: false, reason: 'Not in the library yet' }) },
      ...stub,
    })
    expect(w.find('button').attributes('title')).toBe('Not in the library yet')
  })

  it('marks the active selection', () => {
    const w = mount(ProviderChip, { props: { row: row(), selected: true }, ...stub })
    expect(w.classes().join(' ')).toMatch(/is-selected/)
  })

  it('degraded selectable only in hacker mode', async () => {
    const dr = row({ state: 'degraded', hackerOnly: true, selectable: true })
    const off = mount(ProviderChip, { props: { row: dr, hackerMode: false }, ...stub })
    expect(off.find('button').attributes('disabled')).toBeDefined()
    const on = mount(ProviderChip, { props: { row: dr, hackerMode: true }, ...stub })
    expect(on.find('button').attributes('disabled')).toBeUndefined()
    await on.find('button').trigger('click')
    expect(on.emitted('select')).toBeTruthy()
  })

  it('a forced degraded row is selectable without hacker mode', async () => {
    const dr = row({ state: 'degraded', hackerOnly: true, selectable: true })
    const w = mount(ProviderChip, { props: { row: dr, hackerMode: false, forced: true }, ...stub })
    expect(w.find('button').attributes('disabled')).toBeUndefined()
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })

  it('renders the DEGRADED / RECOVERING badges', () => {
    const deg = mount(ProviderChip, { props: { row: row({ state: 'degraded', hackerOnly: true }) }, ...stub })
    expect(deg.find('[data-test="cap-degraded"]').exists()).toBe(true)
    const rec = mount(ProviderChip, { props: { row: row({ state: 'recovering', hackerOnly: true }) }, ...stub })
    expect(rec.find('[data-test="cap-recovering"]').exists()).toBe(true)
  })

  // --- capability label row (still driven by the ProviderCap decoration) ---
  const cap: ProviderCap = {
    provider: 'gogoanime', display_name: 'GogoAnime', state: 'active', selectable: true,
    hacker_only: false, order: 85, group: 'en', audios: ['sub', 'dub'],
    variants: [
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
      { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ],
  }

  it('renders category + quality tags when cap is present', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap }, ...stub })
    expect(w.findAll('[data-test="cap-cat"]').length).toBe(2)
    expect(w.find('[data-test="cap-quality"]').text()).toContain('1080p')
  })

  it('renders no label row without cap', () => {
    const w = mount(ProviderChip, { props: { row: row() }, ...stub })
    expect(w.find('[data-test="cap-cat"]').exists()).toBe(false)
  })

  it('shows the best pill when best=true', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap, best: true }, ...stub })
    expect(w.find('[data-test="cap-best"]').exists()).toBe(true)
  })
})
