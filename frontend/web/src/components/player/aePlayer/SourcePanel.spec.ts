import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow } from '@/types/aePlayer'

const rows: ProviderRow[] = [
  { def: { id: 'allanime', name: 'AllAnime', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'active' },
  { def: { id: 'animepahe', name: 'AnimePahe', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'disabled', reason: 'Cloudflare challenge' },
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
  it('emits select-provider only for active chips', async () => {
    const w = mount(SourcePanel, { props: { ...baseProps, hackerMode: true } as any })
    await w.find('[data-test="provider-chip"][data-id="allanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['allanime'])
  })
})

describe('SourcePanel collapse', () => {
  const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
  const a = (id: string, state: ProviderRow['state'] = 'active'): ProviderRow =>
    ({ def: { id, name: id, hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state } as ProviderRow)
  const collapseRows = [a('gogoanime'), a('allanime'), a('miruro')]
  const cb = {
    audio: 'sub', lang: 'en', team: null, server: '',
    servers: [] as { id: string; label: string }[], teams: [] as string[],
    rankedIds: ['gogoanime', 'allanime', 'miruro'],
  }

  it('default shows only the top-ranked active provider', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: collapseRows, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('gogoanime')
  })

  it('default shows the selected provider when one is selected', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: collapseRows, provider: 'allanime', hackerMode: false, playbackError: false } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('allanime')
  })

  it('promotes the next active provider when the top one is down', () => {
    const downTop = [a('gogoanime', 'down'), a('allanime'), a('miruro')]
    const w = mount(SourcePanel, { props: { ...cb, rows: downTop, provider: '', hackerMode: false, playbackError: false } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('allanime')
  })

  it('hacker mode shows the full ranked list', () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: collapseRows, provider: 'gogoanime', hackerMode: true, playbackError: false } as any, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })

  it('shows try-another only on playback error, and expands on click', async () => {
    const w = mount(SourcePanel, { props: { ...cb, rows: collapseRows, provider: 'gogoanime', hackerMode: false, playbackError: true } as any, ...mountOpts })
    const btn = w.find('[data-test="try-another"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })
})
