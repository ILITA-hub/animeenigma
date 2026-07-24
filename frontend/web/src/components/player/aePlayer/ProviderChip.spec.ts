import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'

// ProviderChip uses useI18n() in script setup (consolidated badge text);
// stub vue-i18n so tests mount without a real plugin — keys come back as-is.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import type { ProviderRow } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'

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

  // Owner-approved gate (content-verify spec §5): a non-firstparty cap with no
  // verify data is assumed RAW-only and its asserted SUB/DUB category chips
  // are suppressed — no claims without verification. The textual "unverified"
  // marker itself is hidden for now (owner 2026-07-17): the chip just stays
  // badge-plain.
  it('suppresses stream badges (and renders NO unverified marker) when there is no verify data', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap }, ...stub })
    expect(w.findAll('[data-test="cap-badge"]').length).toBe(0)
    expect(w.find('[data-test="cap-unverified"]').exists()).toBe(false)
    // Quality/best badges are independent of the verify gate.
    expect(w.find('[data-test="cap-quality"]').text()).toContain('1080p')
  })

  // Owner fix 2026-07-17: exactly ONE consolidated badge per stream kind —
  // "SUB BURNED-IN RU · DUB RU", never five parallel per-fact chips.
  it('renders one consolidated badge per proven stream kind with langs folded in', () => {
    const verify: ProviderVerify = { status: 'verified', raw: true, dub_langs: ['ru'], hardsub_langs: ['ru'] }
    const w = mount(ProviderChip, { props: { row: row(), cap, verify }, ...stub })
    const badges = w.findAll('[data-test="cap-badge"]')
    expect(badges.length).toBe(2)
    expect(badges[0].text()).toContain('RU')      // sub burned-in RU
    expect(badges[1].text()).toContain('RU')      // dub RU
    expect(w.find('[data-test="cap-quality"]').text()).toContain('1080p')
    expect(w.find('[data-test="cap-unverified"]').exists()).toBe(false)
  })

  it('partial verdict keeps the RAW-assumed sub badge and the proven dub badge', () => {
    const verify: ProviderVerify = { status: 'partial', raw: false, dub_langs: ['ru'], hardsub_langs: [] }
    const w = mount(ProviderChip, { props: { row: row(), cap, verify }, ...stub })
    expect(w.findAll('[data-test="cap-badge"]').length).toBe(2)
  })

  it('renders no label row without cap', () => {
    const w = mount(ProviderChip, { props: { row: row() }, ...stub })
    expect(w.find('[data-test="cap-badge"]').exists()).toBe(false)
  })

  it('shows the best pill when best=true', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap, best: true }, ...stub })
    expect(w.find('[data-test="cap-best"]').exists()).toBe(true)
  })

  // Owner 2026-07-17: every provider shows how many episodes it has ready.
  // The live verify poll's count wins over the (10-min-cached) feed count;
  // 0/absent on both = unknown → hidden.
  it('shows the episodes-ready count, preferring verify over the feed cap', () => {
    const capWithEps = { ...cap, episodes: 12 }
    let w = mount(ProviderChip, { props: { row: row(), cap: capWithEps }, ...stub })
    expect(w.find('[data-test="cap-episodes"]').exists()).toBe(true)

    const verify: ProviderVerify = { status: 'verified', raw: true, dub_langs: [], hardsub_langs: [], episodes: 28 }
    w = mount(ProviderChip, { props: { row: row(), cap: capWithEps, verify }, ...stub })
    const chip = w.getComponent(ProviderChip)
    expect((chip.vm as unknown as { episodesReady: number }).episodesReady).toBe(28)

    w = mount(ProviderChip, { props: { row: row(), cap }, ...stub })
    expect(w.find('[data-test="cap-episodes"]').exists()).toBe(false)
  })

  // Owner 2026-07-24: content-verify rated every probed unit unreachable → a
  // "may not work" warning + red dot, but the source STAYS selectable (live
  // playback in the browser is the real test).
  it('shows the "may not work" badge + red dot but stays selectable when verify.unreachable', async () => {
    const verify: ProviderVerify = { status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [], unreachable: true }
    const w = mount(ProviderChip, { props: { row: row(), cap, verify }, ...stub })
    const badge = w.find('[data-test="cap-unreachable"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('player.sources.mayNotWork')
    // Red dot: the status dot swaps to the destructive token.
    expect(w.find('span.rounded-full').classes()).toContain('bg-destructive')
    // Informational only — still clickable.
    expect(w.find('button').attributes('disabled')).toBeUndefined()
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })

  it('the may-not-work badge takes priority over the roster state badge', () => {
    const verify: ProviderVerify = { status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [], unreachable: true }
    const w = mount(ProviderChip, {
      props: { row: row({ state: 'degraded', hackerOnly: true }), cap, verify },
      ...stub,
    })
    expect(w.find('[data-test="cap-unreachable"]').exists()).toBe(true)
    expect(w.find('[data-test="cap-degraded"]').exists()).toBe(false)
  })

  it('no may-not-work badge when verify is absent or unreachable is false', () => {
    const wNone = mount(ProviderChip, { props: { row: row(), cap }, ...stub })
    expect(wNone.find('[data-test="cap-unreachable"]').exists()).toBe(false)
    const verify: ProviderVerify = { status: 'verified', raw: true, dub_langs: [], hardsub_langs: [], unreachable: false }
    const wOk = mount(ProviderChip, { props: { row: row(), cap, verify }, ...stub })
    expect(wOk.find('[data-test="cap-unreachable"]').exists()).toBe(false)
  })
})
