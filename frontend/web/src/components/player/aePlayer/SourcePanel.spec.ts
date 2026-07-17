import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'

// SourcePanel (and its ProviderChip children) use useI18n() in script setup;
// stub vue-i18n so tests mount without a real plugin — keys come back as-is.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

// Feed-shaped row builder. `order` drives ranking (desc); degraded/no_content
// mirror the old disabled/down buckets in the new single-source-of-truth model.
const r = (id: string, over: Partial<ProviderRow> = {}): ProviderRow => ({
  id, label: id, group: 'en', state: 'active', selectable: true,
  hackerOnly: false, order: 50, audios: ['sub'], ...over,
})

const rows: ProviderRow[] = [
  r('allanime', { order: 90 }),
  r('animepahe', { state: 'degraded', selectable: true, hackerOnly: true, order: 30, reason: 'Cloudflare challenge' }),
]

const baseProps = {
  rows, audio: 'sub', lang: 'en', team: null, provider: 'allanime', server: 's1',
  servers: [{ id: 's1', label: 'Server 1' }], teams: [] as string[],
}

const t = { global: { mocks: { $t: (k: string) => k } } }

describe('SourcePanel', () => {
  it('renders a chip per provider row in hacker mode', () => {
    const w = mount(SourcePanel, { props: { ...baseProps, hackerMode: true } as any, ...t })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(2)
  })
  it('emits update:audio when the Dub slider option is clicked', async () => {
    const w = mount(SourcePanel, { props: baseProps as any, ...t })
    await w.find('[data-test="audio-dub"]').trigger('click')
    expect(w.emitted('update:audio')?.[0]).toEqual(['dub'])
  })
  it('audio slider renders the RAW/DUB labels (translated keys), not SUB/DUB', () => {
    const w = mount(SourcePanel, { props: { ...baseProps, audio: 'sub' } as any, ...t })
    expect(w.find('[data-test="audio-sub"]').text()).toBe('player.aePlayer.audioRaw')
    expect(w.find('[data-test="audio-dub"]').text()).toBe('player.dub')
  })
  it('hides the language slider under RAW; shows EN/RU (no JA) under DUB', () => {
    const raw = mount(SourcePanel, { props: { ...baseProps, audio: 'sub' } as any, ...t })
    expect(raw.find('[data-test="lang-en"]').exists()).toBe(false)
    const dub = mount(SourcePanel, { props: { ...baseProps, audio: 'dub', lang: 'en' } as any, ...t })
    expect(dub.find('[data-test="lang-en"]').exists()).toBe(true)
    expect(dub.find('[data-test="lang-ru"]').exists()).toBe(true)
    expect(dub.find('[data-test="lang-ja"]').exists()).toBe(false)
  })
  it('emits select-provider for an active chip', async () => {
    const w = mount(SourcePanel, { props: { ...baseProps, hackerMode: true } as any, ...t })
    await w.find('[data-test="provider-chip"][data-id="allanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['allanime'])
  })
})

describe('SourcePanel collapse', () => {
  const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
  // order encodes rank: higher = better. gogoanime > allanime > miruro > animepahe > animefever
  const ord: Record<string, number> = { gogoanime: 95, allanime: 90, miruro: 85, animepahe: 80, animefever: 75 }
  const a = (id: string, state: ProviderRow['state'] = 'active', over: Partial<ProviderRow> = {}): ProviderRow =>
    r(id, {
      state, order: ord[id] ?? 50,
      selectable: state === 'active' || state === 'degraded' || state === 'recovering',
      ...over,
    })
  const collapseRows = [a('gogoanime'), a('allanime'), a('miruro')]
  const fiveRows = [a('gogoanime'), a('allanime'), a('miruro'), a('animepahe'), a('animefever')]
  const cb = {
    audio: 'sub', lang: 'en', team: null, server: '',
    servers: [] as { id: string; label: string }[], teams: [] as string[],
  }

  it('default shows the top 3 ranked active providers', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: fiveRows, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['gogoanime', 'allanime', 'miruro'])
  })

  it('pins the selected provider into the visible set even when it ranks below the top 3', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: fiveRows, provider: 'animefever', hackerMode: false, playbackError: false } as any, ...mountOpts })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['gogoanime', 'allanime', 'miruro', 'animefever'])
  })

  it('shows only active providers, excluding no_content/degraded ones', () => {
    const downTop = [a('gogoanime', 'no_content'), a('allanime'), a('miruro')]
    const w = mount(SourcePanel, { props: { ...cb, rows: downTop, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['allanime', 'miruro'])
  })

  it('hacker mode shows the full ranked list', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: collapseRows, provider: 'gogoanime', hackerMode: true, playbackError: false } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })

  it('shows try-another when active providers exceed the top 3 on playback error, and expands on click', async () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: fiveRows, provider: 'gogoanime', hackerMode: false, playbackError: true } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
    const btn = w.find('[data-test="try-another"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(5)
  })

  it('pads the collapsed list to 3 with degraded rows when fewer than 3 are active', () => {
    const rows = [a('gogoanime', 'active'), a('allanime', 'degraded'), a('miruro', 'degraded'), a('animepahe', 'degraded')]
    const w = mount(SourcePanel, { props: { ...cb, rows, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['gogoanime', 'allanime', 'miruro'])
  })

  it('a padded degraded chip is selectable without hacker mode', async () => {
    const rows = [a('gogoanime', 'degraded'), a('allanime', 'degraded')]
    const w = mount(SourcePanel, { props: { ...cb, rows, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    await w.find('[data-test="provider-chip"][data-id="gogoanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['gogoanime'])
  })

  it('sorts active rows above degraded ones, order as tiebreak (hacker mode)', () => {
    // gogoanime has the highest order, but it is degraded → active rows float above it.
    const rows = [a('gogoanime', 'degraded'), a('allanime', 'active'), a('miruro', 'active')]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['allanime', 'miruro', 'gogoanime'])
  })

  it('sorts the degraded bucket by playability_index desc, order as final tiebreak (hacker mode)', () => {
    // All three degraded → within-bucket order is by playability_index desc,
    // NOT by `order` (gogoanime has the highest order but the lowest index).
    const rows = [
      a('gogoanime', 'degraded', { order: 90, playability_index: 0.5 }),
      a('miruro', 'degraded', { order: 10, playability_index: 4.2 }),
      a('nineanime', 'degraded', { order: 50, playability_index: 2.0 }),
    ]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['miruro', 'nineanime', 'gogoanime'])
  })

  it('keeps active bucket ordered by `order` (index does NOT reorder active)', () => {
    const rows = [
      a('allanime', 'active', { order: 10, playability_index: 0.1 }),
      a('miruro', 'active', { order: 90, playability_index: 5.0 }),
    ]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['miruro', 'allanime']) // by order desc, index ignored
  })
})

// ── Stream section + server hygiene (owner fixes 2026-07-17) ────────────────
describe('SourcePanel streams & servers', () => {
  const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
  const verify: ProviderVerify = { status: 'verified', raw: true, dub_langs: ['ru'], hardsub_langs: ['ru'] }

  const kodikCap: ProviderCap = {
    provider: 'kodik', display_name: 'Kodik', state: 'active', selectable: true,
    hacker_only: false, order: 80, group: 'ru', audios: ['sub', 'dub'],
    variants: [
      { category: 'dub', sub_delivery: 'none', qualities: ['720p'], quality_source: 'trait', source: 'trait', team: { name: 'AniDub' } },
      { category: 'sub', sub_delivery: 'hard', qualities: ['720p'], quality_source: 'trait', source: 'trait', team: { name: 'CR.Subs' } },
    ],
  }
  const kodikProps = {
    rows: [r('kodik', { group: 'ru', verify })], audio: 'dub', lang: 'ru', team: 'AniDub',
    provider: 'kodik', server: '', servers: [], teams: ['AniDub', 'CR.Subs'],
    capMap: new Map([['kodik', kodikCap]]),
  }

  it('lists kodik teams as Stream entries with taxonomy badges, between Provider and Server', () => {
    const w = mount(SourcePanel, { props: kodikProps as any, ...mountOpts })
    const entries = w.findAll('[data-test="stream-entry"]')
    expect(entries.length).toBe(2)
    expect(entries[0].text()).toContain('AniDub')
    expect(entries[0].text()).toContain('RU')      // DUB RU badge
    expect(entries[1].text()).toContain('CR.Subs') // SUB BURNED-IN RU badge
    // the section sits AFTER the provider list in the DOM
    const html = w.html()
    expect(html.indexOf('provider-chip')).toBeLessThan(html.indexOf('stream-section'))
  })

  it('selecting a sub-team emits update:team AND syncs audio to sub', async () => {
    const w = mount(SourcePanel, { props: kodikProps as any, ...mountOpts })
    await w.findAll('[data-test="stream-entry"]')[1].trigger('click')
    expect(w.emitted('update:team')?.[0]).toEqual(['CR.Subs'])
    expect(w.emitted('update:audio')?.[0]).toEqual(['sub'])
  })

  it('lists scraper stream kinds as entries and emits update:audio on pick', async () => {
    const cap: ProviderCap = { ...kodikCap, provider: 'miruro', group: 'en', variants: [
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
      { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ] }
    const props = {
      rows: [r('miruro', { verify: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: ['en'] } as ProviderVerify })],
      audio: 'sub', lang: 'en', team: null, provider: 'miruro', server: '', servers: [], teams: [],
      capMap: new Map([['miruro', cap]]),
    }
    const w = mount(SourcePanel, { props: props as any, ...mountOpts })
    const entries = w.findAll('[data-test="stream-entry"]')
    expect(entries.length).toBe(2)
    await entries[1].trigger('click')
    expect(w.emitted('update:audio')?.[0]).toEqual(['dub'])
  })

  it('filters servers to the active stream kind and numbers duplicate labels', () => {
    const props = {
      rows: [r('animepahe')], audio: 'sub', lang: 'en', team: null, provider: 'animepahe', server: '',
      servers: [
        { id: 'k1', label: 'kwik', type: 'sub' }, { id: 'k2', label: 'kwik', type: 'sub' },
        { id: 'k3', label: 'kwik', type: 'sub' }, { id: 'k4', label: 'kwik', type: 'dub' },
        { id: 'k5', label: 'kwik', type: 'dub' }, { id: 'k6', label: 'kwik', type: 'dub' },
      ],
      teams: [],
    }
    const w = mount(SourcePanel, { props: props as any, ...mountOpts })
    const labels = w.findAll('.flex-1.font-semibold.truncate').map(x => x.text()).filter(x => x.startsWith('kwik'))
    expect(labels).toEqual(['kwik 1', 'kwik 2', 'kwik 3'])
  })

  it('hides the Server section when only one server matches the active kind', () => {
    const props = {
      rows: [r('miruro')], audio: 'dub', lang: 'ru', team: null, provider: 'miruro', server: '',
      servers: [{ id: 'kiwi', label: 'kiwi', type: 'dub' }, { id: 'kiwi2', label: 'kiwi', type: 'sub' }],
      teams: [],
    }
    const w = mount(SourcePanel, { props: props as any, ...mountOpts })
    expect(w.text()).not.toContain('Server')
  })
})
