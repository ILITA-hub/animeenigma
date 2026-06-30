import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OtherSubsPanel from './OtherSubsPanel.vue'
import type { GroupedSubs } from '@/types/subtitles'

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

// Configurable helper for provider_health / providers_down tests.
const mountWithData = async (overrides: Partial<GroupedSubs>) => {
  const { subtitlesApi } = await import('@/api/client')
  ;(subtitlesApi.all as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
    data: {
      data: {
        episode: overrides.episode ?? 1,
        languages: overrides.languages ?? {},
        providers_down: overrides.providers_down,
        provider_health: overrides.provider_health,
      },
    },
  })
  const wrapper = mount(OtherSubsPanel, {
    props: { modelValue: true, animeId: 'x', episode: 1, currentTrackUrl: null },
    global: {
      mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) },
      stubs: {
        Modal: { template: '<div><slot /></div>' },
        Badge: { template: '<span><slot /></span>' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

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

  it('switching to a provider without the pinned language resets langFilter to all', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.find('button[data-lang="en"]').trigger('click')
    expect(wrapper.html()).not.toContain('JP')
    expect(wrapper.html()).not.toContain('RU rip')
    // jimaku has only a ja track → 'en' pin is no longer valid → resets to all
    await wrapper.find('button[data-provider="jimaku"]').trigger('click')
    await flushPromises()
    expect(wrapper.html()).toContain('JP')
  })

  it('switching to a provider that still has the pinned language preserves it', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.find('button[data-lang="en"]').trigger('click')
    // opensubtitles has an en track → 'en' pin stays → only EN rip, no RU rip
    await wrapper.find('button[data-provider="opensubtitles"]').trigger('click')
    await flushPromises()
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).not.toContain('RU rip')
  })
})

describe('OtherSubsPanel provider-issues note', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows a merged degraded note from provider_health', async () => {
    const wrapper = await mountWithData({
      languages: {}, episode: 1,
      provider_health: [{ provider: 'jimaku', status: 'degraded' }],
    })
    expect(wrapper.find('[data-testid="provider-issues"]').exists()).toBe(true)
    expect(wrapper.text().toLowerCase()).toContain('jimaku')
  })

  it('merges provider_health (down) with providers_down without duplication', async () => {
    const wrapper = await mountWithData({
      languages: {}, episode: 1,
      provider_health: [{ provider: 'opensubtitles', status: 'down' }],
      providers_down: ['opensubtitles'],
    })
    const matches = wrapper.text().match(/OpenSubtitles/gi) ?? []
    expect(matches.length).toBe(1)
  })

  it('shows no provider-issue note when all healthy', async () => {
    const wrapper = await mountWithData({ languages: {}, episode: 1 })
    expect(wrapper.find('[data-testid="provider-issues"]').exists()).toBe(false)
  })
})
