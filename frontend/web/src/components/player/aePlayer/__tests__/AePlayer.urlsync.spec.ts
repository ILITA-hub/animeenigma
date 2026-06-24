import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import { providerById } from '../providerRegistry'
import type { Combo } from '@/types/aePlayer'

// A `?provider=kodik` deep-link must PIN kodik even though the default filter is
// sub/en (kodik is RU-only) — the fix clamps audio/lang so the row becomes
// relevant. useProviderHealth is mocked to surface kodik as an active row so
// buildAvailable() is non-empty and the resolve/finally → applyInitialProvider
// path runs deterministically (no async smart-default involved).
vi.mock('@/composables/aePlayer/useProviderHealth', () => ({
  useProviderHealth: () => ({
    rows: ref([{ def: providerById('kodik')!, state: 'active' }]),
    start: vi.fn(),
  }),
}))
vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({ capMap: ref(new Map()), rankedIds: ref([]) }),
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

const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, WatchTogetherButton: true,
  SubtitleOverlay: true, ResumePill: true,
}

function mountPlayer(extraProps: Record<string, unknown> = {}) {
  return mount(AePlayer, {
    props: { animeId: 'anime-uuid', anime: { title: 'T', ep: 1, eps: 12 }, theater: false, ...extraProps },
    global: { mocks: { $t: (k: string) => k }, stubs },
  })
}

function readCombo(wrapper: ReturnType<typeof mountPlayer>): Combo {
  const exposed = (wrapper.vm as unknown as { __combo: unknown }).__combo
  const maybeRef = exposed as { value?: Combo }
  return (maybeRef && 'value' in maybeRef && maybeRef.value ? maybeRef.value : exposed) as Combo
}

beforeEach(() => vi.clearAllMocks())

describe('AePlayer — ?provider deep-link pin (cross-language clamp)', () => {
  it('pins kodik AND clamps lang→ru/audio→sub so the RU row becomes selectable', async () => {
    const wrapper = mountPlayer({ initialProvider: 'kodik', initialTeam: 'Studio Band' })
    await flushPromises()
    await nextTick()
    await flushPromises()

    const combo = readCombo(wrapper)
    expect(combo.provider).toBe('kodik')
    expect(combo.lang).toBe('ru')   // clamped from default 'en' — the actual bug fix
    expect(combo.audio).toBe('sub') // kept (kodik serves sub)
    expect(combo.team).toBe('Studio Band')
  })

  it('emits url-sync mirroring the user-pinned source for the shareable URL', async () => {
    const wrapper = mountPlayer({ initialProvider: 'kodik', initialTeam: 'Studio Band' })
    await flushPromises()
    await nextTick()
    await flushPromises()

    const events = wrapper.emitted('url-sync') as Array<[{ provider: string; team: string; episode: number }]> | undefined
    expect(events).toBeTruthy()
    const last = events![events!.length - 1][0]
    expect(last.provider).toBe('kodik')
    expect(last.team).toBe('Studio Band')
    expect(last.episode).toBe(1)
  })
})
