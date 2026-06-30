import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ResumeBanner } from '@/composables/watchState'

// Regression: the in-player resume/airing banner. Before this wiring AePlayer
// hardcoded resumeKind='first-time', so the "next episode not available yet"
// message NEVER surfaced in the primary player; it only worked in the legacy
// Kodik header slot. The parent now passes the unified resume banner down via
// `resumeBanner` (a ResumeBanner), and AePlayer overlays ONLY the airing-status
// family (next-unavailable) — never the just-finished breadcrumb.
const gogoCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 90, group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub' | 'raw')[],
  variants: [],
}
const report: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'ourenglish', providers: [gogoCap] }],
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
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: '', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0),
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

// Render the REAL ResumePill (it's a tiny presentational SFC) so we assert the
// message path end-to-end; i18n's t() is mocked to echo the key, so a rendered
// "anime.resume.notYetAvailable*" proves the airing-status copy is wired.
const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, WatchTogetherButton: true,
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

beforeEach(() => vi.clearAllMocks())

describe('AePlayer — resume/airing banner', () => {
  it('surfaces the not-yet-available message for next-unavailable (no eta)', async () => {
    const resumeBanner: ResumeBanner = { kind: 'next-unavailable', episode: 12 }
    const wrapper = mountPlayer({ resumeBanner })
    await settle()

    const banner = wrapper.find('.pl-airing-status')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('anime.resume.notYetAvailable')
  })

  it('surfaces the eta message for next-unavailable with an etaLabel', async () => {
    const resumeBanner: ResumeBanner = { kind: 'next-unavailable', episode: 12, etaLabel: 'Jul 1' }
    const wrapper = mountPlayer({ resumeBanner })
    await settle()

    const banner = wrapper.find('.pl-airing-status')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('anime.resume.notYetAvailableEta')
  })

  it('does NOT overlay the just-finished breadcrumb over the video', async () => {
    const resumeBanner: ResumeBanner = { kind: 'just-finished', episode: 5 }
    const wrapper = mountPlayer({ resumeBanner })
    await settle()

    expect(wrapper.find('.pl-airing-status').exists()).toBe(false)
  })

  it('shows no banner when no resume state is passed (none / first-time / anonymous)', async () => {
    const wrapper = mountPlayer()
    await settle()

    expect(wrapper.find('.pl-airing-status').exists()).toBe(false)
  })
})
