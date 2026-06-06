import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow } from '@/types/unifiedPlayer'

const rows: ProviderRow[] = [
  { def: { id: 'allanime', name: 'AllAnime', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'active' },
  { def: { id: 'animepahe', name: 'AnimePahe', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'disabled', reason: 'Cloudflare challenge' },
]

const baseProps = {
  rows, audio: 'sub', lang: 'en', team: null, provider: 'allanime', server: 's1',
  servers: [{ id: 's1', label: 'Server 1' }], teams: [] as string[],
}

describe('SourcePanel', () => {
  it('renders a chip per provider row', () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(2)
  })
  it('emits update:audio when the Dub slider option is clicked', async () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    await w.find('[data-test="audio-dub"]').trigger('click')
    expect(w.emitted('update:audio')?.[0]).toEqual(['dub'])
  })
  it('emits select-provider only for active chips', async () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    await w.find('[data-test="provider-chip"][data-id="allanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['allanime'])
  })
})
