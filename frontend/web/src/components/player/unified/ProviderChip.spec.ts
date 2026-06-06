import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow } from '@/types/unifiedPlayer'

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
})
