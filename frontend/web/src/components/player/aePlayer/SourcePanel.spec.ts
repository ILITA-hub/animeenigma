import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow } from '@/types/aePlayer'

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

describe('SourcePanel', () => {
  it('renders a chip per provider row in hacker mode', () => {
    const w = mount(SourcePanel, { props: { ...baseProps, hackerMode: true } as any })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(2)
  })
  it('emits update:audio when the Dub slider option is clicked', async () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    await w.find('[data-test="audio-dub"]').trigger('click')
    expect(w.emitted('update:audio')?.[0]).toEqual(['dub'])
  })
  it('emits select-provider for an active chip', async () => {
    const w = mount(SourcePanel, { props: { ...baseProps, hackerMode: true } as any })
    await w.find('[data-test="provider-chip"][data-id="allanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['allanime'])
  })
})

describe('SourcePanel collapse', () => {
  const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
  // order encodes rank: higher = better. gogoanime > allanime > miruro > animepahe > animefever
  const ord: Record<string, number> = { gogoanime: 95, allanime: 90, miruro: 85, animepahe: 80, animefever: 75 }
  const a = (id: string, state: ProviderRow['state'] = 'active'): ProviderRow =>
    r(id, { state, order: ord[id] ?? 50, selectable: state === 'active' || state === 'degraded' || state === 'recovering' })
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
})
