import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { Combo } from '@/types/aePlayer'

// Regression (feedback 2026-07-10T05-32-46_tNeymik): the "Skip Ending" chip
// rendered BEFORE the user pressed play. Root cause — currentTime.value is
// synced from the <video> element ONLY by the rAF loop (while playing) and the
// final writeProgress() on pause. When a viewer parked in the ending window
// switched episode/server, the incoming source sat paused at 0 while
// currentTime.value still held the OUTGOING source's ~ending-window playhead,
// so activeSkipSegment() reported an active outro and the chip showed pre-play.
// resetPlaybackClock() must zero the reactive playhead at every source swap.
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
    listEpisodes: vi.fn().mockResolvedValue([
      { key: 1, label: 1, number: 1 },
      { key: 2, label: 2, number: 2 },
    ]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: '', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0), lastKnownPlayback: ref(null),
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
// The key mock: a crowdsourced ENDING segment (op left null) so activeSkipSegment
// offers an outro whenever currentTime lands in [1322, 1410).
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({
    opening: ref(null),
    ending: ref({ start: 1322, end: 1411 }),
    loading: ref(false), error: ref(null), refresh: vi.fn(),
  }),
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

// SkipIntroChip is deliberately NOT stubbed so its `v-if="visible"` reflects the
// real skipTarget → [data-test="skip-intro"] presence is the assertion surface.
const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, NextEpisodeCard: true, WatchTogetherButton: true, SubtitleOverlay: true,
  ResumePill: true,
}

function mountPlayer(extraProps: Record<string, unknown> = {}) {
  return mount(AePlayer, {
    props: { animeId: 'anime-uuid', anime: { title: 'T', ep: 1, eps: 12 }, theater: false, ...extraProps },
    global: { mocks: { $t: (k: string) => k }, stubs },
  })
}

async function settle() {
  await flushPromises()
  await nextTick()
  await flushPromises()
}

function readCombo(wrapper: ReturnType<typeof mountPlayer>): Combo {
  const exposed = (wrapper.vm as unknown as { __combo: unknown }).__combo
  const maybeRef = exposed as { value?: Combo }
  return (maybeRef && 'value' in maybeRef && maybeRef.value ? maybeRef.value : exposed) as Combo
}

// Force the <video> element to report a given playhead, then drive the pause
// path (onVideoPause → stopRaf → writeProgress) so currentTime.value syncs from
// it — mirroring a viewer who paused mid-ending on the outgoing source.
async function seekAndPause(wrapper: ReturnType<typeof mountPlayer>, t: number) {
  const el = wrapper.find('video').element as HTMLVideoElement
  Object.defineProperty(el, 'currentTime', { value: t, writable: true, configurable: true })
  Object.defineProperty(el, 'duration', { value: 1420, writable: true, configurable: true })
  await wrapper.find('video').trigger('pause')
  await nextTick()
}

beforeEach(() => vi.clearAllMocks())

describe('AePlayer — skip-ending chip must not leak across a source swap', () => {
  it('hides the Skip Ending chip after an episode change (stale playhead cleared)', async () => {
    const wrapper = mountPlayer()
    await settle()

    // Parked in the ending window on the current source → chip is offered.
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find('[data-test="skip-intro"]').exists()).toBe(true)

    // Switch to a different episode — the new source starts fresh at 0:00.
    await (wrapper.vm as unknown as { onSelectEpisode: (e: { key: number; label: number; number: number }) => void })
      .onSelectEpisode({ key: 2, label: 2, number: 2 })
    await settle()

    // Regression guard: before the fix, currentTime.value still held ~1350 and
    // the chip rendered before the viewer pressed play on episode 2.
    expect(wrapper.find('[data-test="skip-intro"]').exists()).toBe(false)
  })

  it('re-seats the reactive clock on a same-episode swap (preserved ending position, not 0:00)', async () => {
    const wrapper = mountPlayer()
    await settle()

    // Parked in the ending window; reactive clock synced via the pause path.
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find('[data-test="skip-intro"]').exists()).toBe(true)

    // Same-episode server swap → keepPosition path (resolveStreamForCurrentEpisode).
    // resetPlaybackClock zeroes the clock for the swap; capturePlayhead snapshots
    // 1350 (from the element), and restorePlayhead must re-seat currentTime.value.
    readCombo(wrapper).server = 'server-2'
    await settle()
    // jsdom readyState is 0, so restorePlayhead defers its seek to loadedmetadata.
    await wrapper.find('video').trigger('loadedmetadata')
    await nextTick()

    // Guard for the restorePlayhead re-seat (currentTime.value = t): without it the
    // clock would sit at 0 and the chip would wrongly vanish at a preserved ending.
    expect(wrapper.find('[data-test="skip-intro"]').exists()).toBe(true)
  })
})
