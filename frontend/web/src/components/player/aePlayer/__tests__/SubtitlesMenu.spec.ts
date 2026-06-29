import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import SubtitlesMenu from '../SubtitlesMenu.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en } })
const base = {
  subLang: 'off', availableSubLangs: [], langSources: {}, browseCount: 0,
  hardsubNote: null, subSize: 100, subBg: 45, subOffset: 0, autoSync: true,
}
const mountMenu = (props = {}) => mount(SubtitlesMenu, { props: { ...base, ...props }, global: { plugins: [i18n] } })

describe('SubtitlesMenu auto-sync', () => {
  it('emits update:autoSync when the switch is toggled (appearance face)', async () => {
    const w = mountMenu()
    await w.find('[data-test="style-toggle"]').trigger('click')
    await w.find('[data-test="autosync-switch"] button').trigger('click')
    expect(w.emitted('update:autoSync')).toBeTruthy()
  })
  it('shows the VAD debug panel only when autoSyncInfo is provided', async () => {
    const w = mountMenu({ autoSyncInfo: { status: 'locked', offset: 2, confidence: 0.8, events: [
      { delta: 2, confidence: 0.8, windowStart: 0, windowEnd: 12, reason: 'lock' },
    ] } })
    await w.find('[data-test="style-toggle"]').trigger('click')
    expect(w.find('[data-test="autosync-debug"]').exists()).toBe(true)
    expect(w.find('[data-test="autosync-debug"]').text()).toContain('VAD')
  })
  it('hides the debug panel when autoSyncInfo is null', async () => {
    const w = mountMenu({ autoSyncInfo: null })
    await w.find('[data-test="style-toggle"]').trigger('click')
    expect(w.find('[data-test="autosync-debug"]').exists()).toBe(false)
  })
})
