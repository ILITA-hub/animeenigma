import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SubtitlesMenu from './SubtitlesMenu.vue'

// SubtitlesMenu uses $t() in template; stub vue-i18n so tests mount without a
// real plugin — keys/params are returned as a readable string.
import { vi } from 'vitest'
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string, params?: Record<string, unknown>) =>
      params ? `${k} ${JSON.stringify(params)}` : k,
  }),
}))

const base = {
  subLang: 'off',
  availableSubLangs: ['en', 'ja'],
  providerChip: null as { provider: string } | null,
  providerActive: false,
  subSize: 100,
  subBg: 50,
  subOffset: 0,
}

const stubs = { Stepper: true }

describe('SubtitlesMenu', () => {
  it('renders fixed Off/RU/EN/JP fast buttons', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    const labels = w.findAll('[data-test="fast-lang"]').map((b) => b.text())
    expect(labels).toEqual(['RU', 'EN', 'JP'])
    expect(w.find('[data-test="subs-off"]').exists()).toBe(true)
  })

  it('disables a language with no available track (RU here)', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    const ru = w.find('[data-test="fast-lang"][data-lang="ru"]')
    expect(ru.attributes('disabled')).toBeDefined()
    const en = w.find('[data-test="fast-lang"][data-lang="en"]')
    expect(en.attributes('disabled')).toBeUndefined()
  })

  it('emits pick-lang with the language code when an enabled button is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="fast-lang"][data-lang="en"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['en'])
  })

  it('emits pick-lang off when Off is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="subs-off"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['off'])
  })

  it('hides the provider chip when there are no bundled subs', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    expect(w.find('[data-test="sub-provider-chip"]').exists()).toBe(false)
  })

  it('shows the provider chip and emits select-provider when bundled subs exist', async () => {
    const w = mount(SubtitlesMenu, {
      props: { ...base, providerChip: { provider: 'gogoanime' } },
      global: { stubs },
    })
    const chip = w.find('[data-test="sub-provider-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('gogoanime')
    await chip.trigger('click')
    expect(w.emitted('select-provider')).toBeTruthy()
  })

  it('highlights the provider chip (not the lang button) when providerActive', () => {
    const w = mount(SubtitlesMenu, {
      props: { ...base, subLang: 'en', providerChip: { provider: 'gogoanime' }, providerActive: true },
      global: { stubs },
    })
    expect(w.find('[data-test="sub-provider-chip"]').classes().join(' ')).toContain('text-[var(--brand-cyan)]')
    expect(w.find('[data-test="fast-lang"][data-lang="en"]').classes().join(' ')).not.toContain('text-[var(--brand-cyan)]')
  })

  it('emits open-browse', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="open-browse"]').trigger('click')
    expect(w.emitted('open-browse')).toBeTruthy()
  })
})
