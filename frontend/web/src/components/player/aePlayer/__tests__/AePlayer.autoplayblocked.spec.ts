import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

// Autoplay-blocked overlay: a video.play() rejected with NotAllowedError (any
// browser's autoplay policy / blocker extension) must raise the dedicated
// click-to-play overlay + telemetry — NOT be swallowed and NOT count as a dead
// source. Regression guard for the gerahertz reports (2026-07-10) where every
// healthy source was churned through while play() rejections vanished.
const gogoCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 90, group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[],
  variants: [],
}
const report: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'others', providers: [gogoCap] }],
}

vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({
    report: ref(report),
    capMap: ref(new Map<string, ProviderCap>([['gogoanime', gogoCap]])),
  }),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn().mockResolvedValue([{ key: 1, label: 1, number: 1 }]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: 'https://example.test/playlist.m3u8', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), lastKnownPlayback: ref(null),
    load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0), fragLoadedCount: ref(0), videoCodec: ref(''),
  }),
}))
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: vi.fn().mockResolvedValue(undefined), resolvedCombo: ref(null) }),
}))
vi.mock('@/composables/aePlayer/useWatchTracking', () => ({
  useWatchTracking: () => ({
    maxTime: ref(0), episodeMarked: ref(false), marking: ref(false), onTick: vi.fn(),
    saveNow: vi.fn(), beaconSave: vi.fn(), markWatched: vi.fn().mockResolvedValue(undefined), resetEpisode: vi.fn(),
  }),
}))
vi.mock('@/composables/aePlayer/usePlaybackStats', () => ({ usePlaybackStats: () => ({ stats: ref(null), sample: vi.fn() }) }))
vi.mock('@/composables/useWatchedEpisodes', () => ({ useWatchedEpisodes: () => ({ watchedUpTo: ref(0), refresh: vi.fn().mockResolvedValue(undefined) }) }))
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({ opening: ref(null), ending: ref(null), loading: ref(false), error: ref(null), refresh: vi.fn() }),
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false, user: null }) }))
vi.mock('@/stores/viewerContext', () => ({ useViewerContextStore: () => ({ whenLoaded: vi.fn().mockResolvedValue(null) }) }))
vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn().mockResolvedValue({ data: { data: null } }) },
  aeApi: { getEpisodes: vi.fn().mockResolvedValue({ data: { data: { available: false, episodes: [] } } }) },
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [] } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [] } } }),
    getStream: vi.fn().mockResolvedValue({ data: { data: { stream: { sources: [] } } } }),
  },
}))
vi.mock('@/utils/playerTelemetry', () => ({ recordPlayerEvent: vi.fn() }))

import AePlayer from '../AePlayer.vue'
import { recordPlayerEvent } from '@/utils/playerTelemetry'

const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, NextEpisodeChip: true, WatchTogetherButton: true,
  SubtitleOverlay: true,
}

function mountPlayer(extraProps: Record<string, unknown> = {}) {
  return mount(AePlayer, {
    props: { animeId: 'anime-uuid', anime: { title: 'T', eps: 12 }, theater: false, ...extraProps },
    global: { mocks: { $t: (k: string) => k }, stubs },
  })
}

async function settle() {
  await flushPromises()
  await nextTick()
  await flushPromises()
}

function rejectionError(name: string): Error {
  return Object.assign(new Error(`play() vetoed (${name})`), { name })
}

/** Stub the real <video> element's play() and trigger the big-play-button path. */
async function clickPlayWith(wrapper: ReturnType<typeof mountPlayer>, play: () => Promise<void>) {
  const video = wrapper.find('video').element as HTMLVideoElement
  Object.defineProperty(video, 'paused', { configurable: true, value: true })
  video.play = vi.fn(play)
  wrapper.getComponent({ name: 'BigPlayButton' }).vm.$emit('play')
  await settle()
  return video
}

beforeEach(() => vi.clearAllMocks())

describe('AePlayer — autoplay-blocked overlay', () => {
  it('raises the overlay + telemetry when play() rejects with NotAllowedError', async () => {
    const wrapper = mountPlayer()
    await settle()

    await clickPlayWith(wrapper, () => Promise.reject(rejectionError('NotAllowedError')))

    expect(wrapper.find('[data-test="autoplay-blocked-play"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('player.aePlayer.autoplayBlocked')
    // No hint yet — first rejection only.
    expect(wrapper.text()).not.toContain('player.aePlayer.autoplayBlockedHint')
    expect(recordPlayerEvent).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'playback_start_rejected', error_kind: 'NotAllowedError' }),
    )
  })

  it('shows the browser-permission hint when the overlay retry is vetoed too', async () => {
    const wrapper = mountPlayer()
    await settle()

    const video = await clickPlayWith(wrapper, () => Promise.reject(rejectionError('NotAllowedError')))

    const retry = wrapper.find('[data-test="autoplay-blocked-play"]')
    expect(retry.exists()).toBe(true)
    await retry.trigger('click')
    await settle()

    expect(wrapper.text()).toContain('player.aePlayer.autoplayBlockedHint')
    expect(video.play).toHaveBeenCalledTimes(2)
    // Telemetry stays one event per resolve — a retry veto is the same block.
    const rejectedCalls = (recordPlayerEvent as ReturnType<typeof vi.fn>).mock.calls
      .filter(([e]) => (e as { kind: string }).kind === 'playback_start_rejected')
    expect(rejectedCalls).toHaveLength(1)
  })

  it('clears the overlay when a retry succeeds', async () => {
    const wrapper = mountPlayer()
    await settle()

    await clickPlayWith(wrapper, () => Promise.reject(rejectionError('NotAllowedError')))
    const retry = wrapper.find('[data-test="autoplay-blocked-play"]')
    const video = wrapper.find('video').element as HTMLVideoElement
    video.play = vi.fn(() => Promise.resolve())
    await retry.trigger('click')
    await settle()

    expect(wrapper.find('[data-test="autoplay-blocked-play"]').exists()).toBe(false)
  })

  it('ignores benign AbortError rejections (source-swap lifecycle noise)', async () => {
    const wrapper = mountPlayer()
    await settle()

    await clickPlayWith(wrapper, () => Promise.reject(rejectionError('AbortError')))

    expect(wrapper.find('[data-test="autoplay-blocked-play"]').exists()).toBe(false)
    const rejectedCalls = (recordPlayerEvent as ReturnType<typeof vi.fn>).mock.calls
      .filter(([e]) => (e as { kind: string }).kind === 'playback_start_rejected')
    expect(rejectedCalls).toHaveLength(0)
  })
})
