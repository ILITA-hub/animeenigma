import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

// Task 3b wiring smoke check: a single AUTO-selected provider that always
// fails to resolve must exhaust the failover chain on the very first attempt
// and emit exactly one playback_failed telemetry event carrying the
// diagnostic bundle. The failure CLASSIFICATION logic itself is already
// fully unit-tested in playbackFailure.spec.ts (Task 3a) — this test only
// verifies AePlayer.vue actually calls recordPlayerEvent with the right shape.

const recordSpy = vi.fn()
vi.mock('@/utils/playerTelemetry', () => ({
  recordPlayerEvent: (...a: unknown[]) => recordSpy(...a),
  flushPlayerTelemetry: vi.fn(),
}))

const gogoCap: ProviderCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 100, group: 'en' as const, audios: ['sub'] as ('sub' | 'dub')[],
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
    resolveStream: vi.fn().mockRejectedValue(new Error('boom')), // always fails
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
// Same engine shape as AePlayer.aeLangRouting.spec.ts, plus the edge/fragment
// fields buildDiagnosticBundle() reads on the failure path (unused there
// because that spec's resolveStream never fails).
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), lastKnownPlayback: ref(null),
    load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0),
    servedEdge: ref(''), edgeTrail: ref(''), fragLoadedCount: ref(0), videoCodec: ref(''),
  }),
}))
const resolveSpy = vi.fn().mockResolvedValue(undefined)
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: resolveSpy, resolvedCombo: ref(null) }),
}))
vi.mock('@/composables/aePlayer/useWatchTracking', () => ({
  useWatchTracking: () => ({
    maxTime: ref(0), episodeMarked: ref(false), marking: ref(false), onTick: vi.fn(),
    saveNow: vi.fn(), beaconSave: vi.fn(), markWatched: vi.fn().mockResolvedValue(undefined), resetEpisode: vi.fn(),
  }),
}))
// Real usePlaybackStats never yields a null stats ref (defaults to a zeroed
// PlaybackStats object) — buildDiagnosticBundle relies on that contract.
vi.mock('@/composables/aePlayer/usePlaybackStats', () => ({
  usePlaybackStats: () => ({
    stats: ref({
      readyState: 0, bufferAheadSec: 0, bufferBehindSec: 0,
      droppedFrames: 0, totalFrames: 0, resolution: '',
    }),
    sample: vi.fn(),
  }),
}))
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

import AePlayer from '../AePlayer.vue'

const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, NextEpisodeChip: true, WatchTogetherButton: true,
  SubtitleOverlay: true, ResumePill: true,
}

function mountPlayer(extraProps: Record<string, unknown> = {}) {
  return mount(AePlayer, {
    props: { animeId: 'anime-uuid', anime: { title: 'T', ep: 1, eps: 12 }, theater: false, ...extraProps },
    global: { mocks: { $t: (k: string) => k }, stubs },
  })
}

beforeEach(() => {
  recordSpy.mockClear()
  resolveSpy.mockClear()
  resolveSpy.mockResolvedValue(undefined)
})

describe('AePlayer terminal playback-failure telemetry', () => {
  it('emits playback_failed with reason all_exhausted when the only provider fails', async () => {
    mountPlayer()
    await flushPromises()
    await nextTick()
    await flushPromises()
    await nextTick()
    await flushPromises()

    const failed = recordSpy.mock.calls
      .map((c) => c[0] as { kind: string; provider: string; detail?: Record<string, unknown> })
      .filter((e) => e.kind === 'playback_failed')

    expect(failed.length).toBe(1)
    expect(failed[0].provider).toBe('gogoanime')
    expect(failed[0].detail?.reason).toBe('all_exhausted')
    expect(failed[0].detail?.all_exhausted).toBe(true)
    expect(failed[0].detail?.engine).toBeTruthy()
  })
})
