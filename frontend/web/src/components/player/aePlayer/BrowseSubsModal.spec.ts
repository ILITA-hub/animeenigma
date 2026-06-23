import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BrowseSubsModal from './BrowseSubsModal.vue'

// BrowseSubsModal uses $t() in template; stub vue-i18n so tests mount without
// a real plugin — keys are returned as-is (good enough for DOM presence checks).
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

const tracks = [
  { url: 't1', provider: 'Jimaku', lang: 'ja', label: 'Kawaisubs Re:Zero 12', format: 'ass' },
  { url: 't2', provider: 'OpenSubtitles', lang: 'en', label: 'Re:Zero 12 HorribleSubs', format: 'srt' },
]

describe('BrowseSubsModal', () => {
  it('groups tracks by language', () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    expect(w.findAll('[data-test="lang-group"]').length).toBe(2)
  })
  it('search narrows the visible tracks', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    await w.find('[data-test="search"]').setValue('horriblesubs')
    expect(w.findAll('[data-test="track"]').length).toBe(1)
  })
  it('emits select with the track', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    await w.find('[data-test="track"] [data-test="select"]').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })
  it('closes on Escape', () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(w.emitted('close')).toBeTruthy()
    w.unmount()
  })

  it('shows a loading state', () => {
    const w = mount(BrowseSubsModal, { props: { tracks: [], selectedUrl: null, loading: true } })
    expect(w.find('[data-test="subs-loading"]').exists()).toBe(true)
  })

  it('shows an error with a retry button that emits retry', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks: [], selectedUrl: null, error: 'jimaku down' } })
    expect(w.find('[data-test="subs-error"]').exists()).toBe(true)
    await w.find('[data-test="subs-retry"]').trigger('click')
    expect(w.emitted('retry')).toBeTruthy()
  })

  it('emits off when "Subtitles off" is clicked', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks: [{ url: 'u', provider: 'jimaku', lang: 'ja', label: 'L', format: 'srt' }], selectedUrl: 'u' } })
    await w.find('[data-test="subs-off"]').trigger('click')
    expect(w.emitted('off')).toBeTruthy()
  })

  const mixed = [
    { url: 'en1', provider: 'opensubtitles', lang: 'en', label: 'EN', format: 'srt' },
    { url: 'de1', provider: 'opensubtitles', lang: 'de', label: 'DE', format: 'srt' },
  ]

  it('hides non RU/EN/JP language groups by default', () => {
    const w = mount(BrowseSubsModal, { props: { tracks: mixed, selectedUrl: null } })
    const langs = w.findAll('[data-test="lang-group"]').map((g) => g.find('h3').text())
    expect(langs.some((l) => l.startsWith('EN'))).toBe(true)
    expect(langs.some((l) => l.startsWith('DE'))).toBe(false)
    expect(w.find('[data-test="more-languages"]').exists()).toBe(true)
  })

  it('reveals other-language groups when "More languages" is clicked', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks: mixed, selectedUrl: null } })
    await w.find('[data-test="more-languages"]').trigger('click')
    const langs = w.findAll('[data-test="lang-group"]').map((g) => g.find('h3').text())
    expect(langs.some((l) => l.startsWith('DE'))).toBe(true)
  })

  it('does not show "More languages" when only RU/EN/JP exist', () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    expect(w.find('[data-test="more-languages"]').exists()).toBe(false)
  })
})
