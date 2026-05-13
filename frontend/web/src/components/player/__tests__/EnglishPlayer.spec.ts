import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { nextTick } from 'vue'
import EnglishPlayer from '../EnglishPlayer.vue'

// SCRAPER-HEAL-08 — three-phase loader Vitest coverage.
// 9 cases: 3 phases × 2 locales (6) + precedence (1) + meta.gated:true (1) + meta.gated absent (1).

// Stub all external/network/runtime modules that the component pulls in via
// `<script setup>`. We don't render real network, real auth, or real video.js;
// we only need the template to evaluate phase-text + the fetchStream branch
// to toggle validatingStream.
vi.mock('@/api/client', () => ({
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [], meta: { tried: [] } } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [], meta: { tried: [] } } } }),
    getStream: vi.fn(),
    getHealth: vi.fn().mockResolvedValue({ data: { providers: {} } }),
  },
  jimakuApi: { getSubtitles: vi.fn().mockResolvedValue({ data: { entries: [] } }) },
  userApi: {
    saveWatchHistory: vi.fn().mockResolvedValue({ data: {} }),
    updateProgress: vi.fn().mockResolvedValue({ data: {} }),
  },
}))

// Stub auth store (the component reads from it but tests don't need real auth).
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAuthenticated: false,
    user: null,
    token: null,
  }),
}))

// Stub video.js + hls.js — the component imports them but tests never reach
// initPlayer() because we only drive loader-state, not real playback.
vi.mock('video.js', () => ({ default: vi.fn(() => ({ dispose: vi.fn(), on: vi.fn(), ready: vi.fn() })) }))
vi.mock('hls.js', () => ({ default: class { static isSupported() { return false } destroy() {} } }))

function makeI18n(locale: 'en' | 'ru') {
  return createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'en',
    messages: {
      en: { player: {} },
      ru: { player: {} },
    },
    // Missing-key handler returns the key itself — the component calls
    // t('player.foo') for various non-loader strings; we don't assert those.
    missing: (_l, key) => key,
    silentTranslationWarn: true,
    silentFallbackWarn: true,
  })
}

const baseProps = {
  animeId: 'test-anime-id',
  malId: '52991',
  shikimoriId: '52991',
  title: 'Frieren',
  posterUrl: '',
} as Record<string, unknown>

async function mountAt(locale: 'en' | 'ru') {
  const i18n = makeI18n(locale)
  const wrapper = mount(EnglishPlayer, {
    global: {
      plugins: [i18n],
      // Stub heavy child components — SubtitleOverlay etc. — that aren't part
      // of the loader-phase contract.
      stubs: {
        SubtitleOverlay: true,
        ReportButton: true,
      },
    },
    props: baseProps as never,
  })
  await flushPromises()
  await nextTick()
  // Seed an episode so the `episodes.length === 0` early-return branch in the
  // template doesn't hide the loader overlay. The exact episode object is only
  // used by template v-for over the picker; the loader phase logic doesn't
  // touch episode contents.
  const vm = wrapper.vm as unknown as { episodes: unknown[] }
  vm.episodes = [{ number: 1, id: 'ep1' }]
  await nextTick()
  return wrapper
}

// vue-test-utils `wrapper.vm` is a proxy that AUTO-UNWRAPS exposed refs —
// i.e. `wrapper.vm.loadingServers` returns the bare `boolean`, not the Ref.
// Setting `wrapper.vm.loadingServers = true` writes through to the underlying
// ref's `.value`. So in tests we treat the exposed surface as plain props.
interface ExposedRefs {
  loadingServers: boolean
  loadingStream: boolean
  validatingStream: boolean
  selectedEpisode: unknown
  selectedServer: unknown
  episodes: unknown[]
  fetchStream?: () => Promise<void>
}

function exposed(wrapper: ReturnType<typeof mount>): ExposedRefs {
  return wrapper.vm as unknown as ExposedRefs
}

describe('EnglishPlayer three-phase loader (SCRAPER-HEAL-08)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it.each([
    ['en', 'Looking up sources…',           'loadingServers'],
    ['en', 'Connecting to remote stream…',  'loadingStream'],
    ['en', 'Verifying playback…',           'validatingStream'],
    ['ru', 'Поиск источников…',             'loadingServers'],
    ['ru', 'Подключение к удалённому потоку…', 'loadingStream'],
    ['ru', 'Проверка воспроизведения…',     'validatingStream'],
  ])('locale=%s renders %s when %s is true', async (locale, expected, refName) => {
    const wrapper = await mountAt(locale as 'en' | 'ru')
    const vm = exposed(wrapper)
    // Phase 1 needs selectedEpisode too (existing condition guards against
    // showing the overlay before episode pick).
    if (refName === 'loadingServers') {
      vm.selectedEpisode = { number: 1, id: 'ep1' }
    }
    (vm as unknown as Record<string, unknown>)[refName] = true
    await nextTick()
    expect(wrapper.text()).toContain(expected)
  })

  it('precedence: validatingStream wins over loadingStream + loadingServers', async () => {
    const wrapper = await mountAt('en')
    const vm = exposed(wrapper)
    vm.loadingServers = true
    vm.loadingStream = true
    vm.validatingStream = true
    vm.selectedEpisode = { number: 1, id: 'ep1' }
    await nextTick()
    expect(wrapper.text()).toContain('Verifying playback…')
    expect(wrapper.text()).not.toContain('Connecting to remote stream…')
    expect(wrapper.text()).not.toContain('Looking up sources…')
  })

  it('meta.gated=true sets validatingStream during fetchStream (then clears in finally)', async () => {
    const { scraperApi } = await import('@/api/client')
    ;(scraperApi.getStream as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          stream: { sources: [{ url: 'https://example.test/x.m3u8', type: 'hls' }], tracks: [], headers: {} },
          meta: { tried: ['gogoanime'], gated: true },
        },
      },
    })
    const wrapper = await mountAt('en')
    const vm = exposed(wrapper)
    vm.selectedEpisode = { number: 1, id: 'ep1' }
    vm.selectedServer = { id: 'streamhg', name: 'StreamHG', type: 'sub' }

    // Capture validatingStream values during the fetchStream lifecycle via a
    // synchronous watch — we expect it to flip to true at least once before
    // landing back at false in the finally block.
    const { watch } = await import('vue')
    const seen: boolean[] = []
    const stop = watch(
      () => vm.validatingStream,
      (v) => { seen.push(v) },
      { flush: 'sync' },
    )

    if (typeof vm.fetchStream === 'function') {
      await vm.fetchStream()
    }
    stop()

    // Verify the gated branch toggled the ref to true at least once.
    expect(seen).toContain(true)
    // And it ended up false (cleared in `finally`).
    expect(vm.validatingStream).toBe(false)
  })

  it('meta.gated absent does NOT toggle validatingStream', async () => {
    const { scraperApi } = await import('@/api/client')
    ;(scraperApi.getStream as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          stream: { sources: [{ url: 'https://example.test/x.m3u8', type: 'hls' }], tracks: [], headers: {} },
          meta: { tried: ['gogoanime'] }, // no `gated` key — Wave-1 cache-hit shape
        },
      },
    })
    const wrapper = await mountAt('en')
    const vm = exposed(wrapper)
    vm.selectedEpisode = { number: 1, id: 'ep1' }
    vm.selectedServer = { id: 'streamhg', name: 'StreamHG', type: 'sub' }

    const { watch } = await import('vue')
    const seenTrue: boolean[] = []
    const stop = watch(
      () => vm.validatingStream,
      (v) => { if (v) seenTrue.push(true) },
      { flush: 'sync' },
    )

    if (typeof vm.fetchStream === 'function') {
      await vm.fetchStream()
    }
    stop()

    expect(seenTrue).toHaveLength(0)
    expect(vm.validatingStream).toBe(false)
  })
})
