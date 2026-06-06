import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BrowseSubsModal from './BrowseSubsModal.vue'

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
})
