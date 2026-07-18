import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { WatchCombo } from '@/types/preference'

// Phase C source-panel truth: a real self-hosted ("ae") English dub must
// route ONLY under DUB/EN, not under every language the `firstparty` group
// nominally serves (en/ru/ja). Before this fix, AePlayer's buildAvailable()
// derived a cap's routable languages purely from GROUP_LANGS[cap.group] —
// for `firstparty` that's ['en','ru','ja'] regardless of what the title
// actually has. So an ae title with ONLY an English dub would enumerate
// bogus dub:ru and dub:ja combos too. The fix reads the real per-title
// `cap.lang` (set by aeFamily from AeInfo) when present, so the enumerated
// combo set matches reality.
const aeDubEnCap = {
  provider: 'ae', display_name: 'AnimeEnigma',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 100, group: 'firstparty' as const, audios: ['dub'] as ('sub' | 'dub')[],
  lang: 'en' as const,
  variants: [],
}
const aeReport: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'aeProvider', providers: [aeDubEnCap] }],
}

const resolveSpy = vi.fn().mockResolvedValue(undefined)

vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({
    report: ref(aeReport),
    capMap: ref(new Map<string, ProviderCap>([['ae', aeDubEnCap]])),
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
    fragStats: ref([]), bandwidthEstimate: ref(0), fragLoadedCount: ref(0), videoCodec: ref(''),
  }),
}))
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: resolveSpy, resolvedCombo: ref(null) }),
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
  vi.clearAllMocks()
  resolveSpy.mockResolvedValue(undefined)
})

describe('AePlayer — ae dub cap.lang scopes combo routing', () => {
  it('enumerates only dub:en for an ae cap real-dubbed in English (not dub:ru/dub:ja)', async () => {
    mountPlayer()
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(resolveSpy).toHaveBeenCalled()
    const available = resolveSpy.mock.calls[0][0] as WatchCombo[]
    const aeCombos = available.filter((c) => c.player === 'ae')
    expect(aeCombos.map((c) => `${c.watch_type}:${c.language}`).sort()).toEqual(['dub:en'])
  })
})
