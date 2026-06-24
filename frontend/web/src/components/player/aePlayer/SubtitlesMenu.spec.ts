import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SubtitlesMenu from './SubtitlesMenu.vue'

// SubtitlesMenu uses $t() in template; stub vue-i18n so tests mount without a
// real plugin — keys/params are returned as a readable string.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string, params?: Record<string, unknown>) =>
      params ? `${k} ${JSON.stringify(params)}` : k,
  }),
}))

const base = {
  subLang: 'off',
  availableSubLangs: ['en', 'ja'],
  langSources: { en: 'SubsPlease', ja: 'Kitsunekko' } as Record<string, string>,
  subSize: 100,
  subBg: 50,
  subOffset: 0,
}

const stubs = { Stepper: true }

describe('SubtitlesMenu', () => {
  it('renders Off plus three native language rows (RU/EN/JP)', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    expect(w.find('[data-test="subs-off"]').exists()).toBe(true)
    const langs = w.findAll('[data-test="lang-row"]').map((b) => b.attributes('data-lang'))
    expect(langs).toEqual(['ru', 'en', 'ja'])
    // Native autonyms, not codes
    const ru = w.find('[data-test="lang-row"][data-lang="ru"]')
    expect(ru.text()).toContain('Русский')
  })

  it('disables a language with no available track (RU here)', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    const ru = w.find('[data-test="lang-row"][data-lang="ru"]')
    expect(ru.attributes('disabled')).toBeDefined()
    const en = w.find('[data-test="lang-row"][data-lang="en"]')
    expect(en.attributes('disabled')).toBeUndefined()
  })

  it('shows the source label as row meta for an available language', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    expect(w.find('[data-test="lang-row"][data-lang="en"]').text()).toContain('SubsPlease')
  })

  it('emits pick-lang with the language code when an enabled row is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="lang-row"][data-lang="en"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['en'])
  })

  it('does not emit pick-lang for a disabled row', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="lang-row"][data-lang="ru"]').trigger('click')
    expect(w.emitted('pick-lang')).toBeFalsy()
  })

  it('emits pick-lang off when Off is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="subs-off"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['off'])
  })

  it('toggles to the appearance face and back', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    // Captions face shows language rows; appearance controls hidden
    expect(w.find('[data-test="sub-size"]').exists()).toBe(false)
    await w.find('[data-test="style-toggle"]').trigger('click')
    expect(w.find('[data-test="sub-size"]').exists()).toBe(true)
    expect(w.find('[data-test="lang-row"]').exists()).toBe(false)
    await w.find('[data-test="style-back"]').trigger('click')
    expect(w.find('[data-test="lang-row"]').exists()).toBe(true)
  })

  it('emits update:subSize when the size slider changes', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="style-toggle"]').trigger('click')
    const size = w.find('[data-test="sub-size"]')
    ;(size.element as HTMLInputElement).value = '150'
    await size.trigger('input')
    expect(w.emitted('update:subSize')?.[0]).toEqual([150])
  })

  it('emits open-browse', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="open-browse"]').trigger('click')
    expect(w.emitted('open-browse')).toBeTruthy()
  })
})
