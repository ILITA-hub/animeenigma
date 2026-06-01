import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OtherSubsPanel from './OtherSubsPanel.vue'

// The component reads `useI18n().t`/`locale` in <script> (providerLabel,
// languageHeader, orderLangs) AND `$t` in <template> (filter labels), so BOTH
// mocks below are intentional — do not "dedupe" them. With t:(k)=>k, label
// lookups fall back to provider key / uppercased lang code, which is why the
// chip selectors below target data-* attributes, not rendered text.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }),
}))

vi.mock('@/api/client', () => ({
  subtitlesApi: {
    all: vi.fn().mockResolvedValue({
      data: {
        data: {
          episode: 1,
          languages: {
            ja: [{ url: 'https://jimaku.cc/a.srt', lang: 'ja', label: 'JP', provider: 'jimaku', format: 'srt' }],
            en: [{ url: '/api/anime/x/subtitles/opensubtitles/file/1', lang: 'en', label: 'EN rip', provider: 'opensubtitles', format: 'srt' }],
            ru: [{ url: '/api/anime/x/subtitles/opensubtitles/file/2', lang: 'ru', label: 'RU rip', provider: 'opensubtitles', format: 'srt' }],
          },
        },
      },
    }),
  },
}))

const mountPanel = () =>
  mount(OtherSubsPanel, {
    props: { modelValue: true, animeId: 'x', episode: 1, currentTrackUrl: null },
    global: {
      mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) },
      stubs: {
        Modal: { template: '<div><slot /></div>' },
        Badge: { template: '<span><slot /></span>' },
      },
    },
  })

describe('OtherSubsPanel filters', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows all three languages by default', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.html()).toContain('JP')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).toContain('RU rip')
  })

  it('provider filter = opensubtitles hides the Jimaku (ja) track', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const osBtn = wrapper.find('button[data-provider="opensubtitles"]')
    expect(osBtn.exists()).toBe(true)
    await osBtn.trigger('click')
    expect(wrapper.html()).not.toContain('JP')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).toContain('RU rip')
  })

  it('language filter = en narrows to the English track only', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const enChip = wrapper.find('button[data-lang="en"]')
    expect(enChip.exists()).toBe(true)
    await enChip.trigger('click')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).not.toContain('RU rip')
  })
})
