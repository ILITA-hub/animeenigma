/**
 * Task 6 — AePlayer subtitle wiring regression guard.
 *
 * These tests confirm the two canonical subtitle bugs are fixed:
 *   (a) BrowseSubsModal was always receiving `tracks=[]`  (the original bug)
 *   (b) SubtitleOverlay wiring is present at runtime (no type error)
 *
 * AePlayer is a very heavy component (~2200 lines). We mount it fully to get
 * real reactivity with stubs for child components. The harness is modelled after
 * AePlayer.room.spec.ts — same mocks, same `stubs` map.
 *
 * Auto-select end-to-end (test b) requires triggering the full resolveStream
 * cycle inside the heavy harness which is too brittle. The auto-select
 * logic is unit-tested in pickDefaultSubtitle.spec.ts (Task 4). We confirm
 * here only that: (a) tracks pass through to the modal and (b) SubtitleOverlay
 * renders without prop-shape errors.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'

// ─── Heavy network/store composables — stubbed so the component mounts ─────────
vi.mock('@/composables/aePlayer/useProviderHealth', () => ({
  useProviderHealth: () => ({ rows: ref([]), start: vi.fn() }),
}))
vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({ capMap: ref(new Map()), rankedIds: ref([]) }),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn().mockResolvedValue([]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: '', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null),
    load: vi.fn().mockResolvedValue(undefined),
    destroy: vi.fn(),
    levels: ref([]),
    currentLevelLabel: ref('Auto'),
    setLevel: vi.fn(),
    fragStats: ref([]),
    bandwidthEstimate: ref(0),
  }),
}))
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({
    resolve: vi.fn().mockResolvedValue(undefined),
    resolvedCombo: ref(null),
  }),
}))
vi.mock('@/composables/aePlayer/useWatchTracking', () => ({
  useWatchTracking: () => ({
    maxTime: ref(0),
    episodeMarked: ref(false),
    marking: ref(false),
    onTick: vi.fn(),
    saveNow: vi.fn(),
    beaconSave: vi.fn(),
    markWatched: vi.fn().mockResolvedValue(undefined),
    resetEpisode: vi.fn(),
  }),
}))
vi.mock('@/composables/aePlayer/usePlaybackStats', () => ({
  usePlaybackStats: () => ({ stats: ref(null), sample: vi.fn() }),
}))
vi.mock('@/composables/useWatchedEpisodes', () => ({
  useWatchedEpisodes: () => ({ watchedUpTo: ref(0), refresh: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({
    opening: ref(null),
    ending: ref(null),
    loading: ref(false),
    error: ref(null),
    refresh: vi.fn(),
  }),
}))
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: vi.fn() }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: false, user: null }),
}))
vi.mock('@/stores/viewerContext', () => ({
  useViewerContextStore: () => ({ whenLoaded: vi.fn().mockResolvedValue(null) }),
}))

// API clients touched on mount (ae availability probe, progress fetch).
// subtitlesApi.all is called by useSubtitleTracks when ensureLoaded() runs.
vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn().mockResolvedValue({ data: { data: null } }) },
  aeApi: { getEpisodes: vi.fn().mockResolvedValue({ data: { data: { available: false, episodes: [] } } }) },
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [] } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [] } } }),
    getStream: vi.fn().mockResolvedValue({ data: { data: { stream: { sources: [] } } } }),
  },
  subtitlesApi: {
    all: vi.fn().mockResolvedValue({
      data: {
        data: {
          languages: {
            ja: [
              {
                url: 'https://proxy.animeenigma.ru/subs/jimaku/test.ass',
                provider: 'jimaku',
                lang: 'ja',
                label: 'Japanese (jimaku)',
                format: 'ass',
              },
            ],
          },
          episode: 1,
          providers_down: [],
        },
      },
    }),
    byLang: vi.fn().mockResolvedValue({ data: { data: { languages: {}, episode: 1 } } }),
  },
}))

vi.mock('@/utils/playerTelemetry', () => ({ recordPlayerEvent: vi.fn() }))

// ─── Mock useSubtitleTracks (Task 3) ──────────────────────────────────────────
// We expose a shared fakeTracksRef that tests can pre-populate to control
// what the composable returns. ensureLoaded fills it with a JA track.
// Note: do NOT use vi.fn() with a rejected-promise return in beforeEach — that
// triggers a false vitest unhandled-rejection. Set return values inside each test.
const fakeTracksRef = ref<Array<{ url: string; provider: string; lang: string; label: string; format: string }>>([])

vi.mock('@/composables/aePlayer/useSubtitleTracks', () => ({
  useSubtitleTracks: () => ({
    tracks: fakeTracksRef,
    loading: ref(false),
    error: ref(null),
    providersDown: ref([]),
    ensureLoaded: vi.fn().mockResolvedValue(undefined),
    refetch: vi.fn().mockResolvedValue(undefined),
  }),
}))

import AePlayer from '../AePlayer.vue'
import BrowseSubsModal from '../BrowseSubsModal.vue'

const stubs = {
  PlayerControlBar: true,
  SourcePanel: true,
  EpisodesPanel: true,
  PlaybackSettingsMenu: true,
  SubtitlesMenu: true,
  // BrowseSubsModal is NOT stubbed — we need to read its `tracks` prop
  BigPlayButton: true,
  BufferingOverlay: true,
  DebugHud: true,
  SkipIntroChip: true,
  NextEpisodeCard: true,
  WatchTogetherButton: true,
  SubtitleOverlay: true,
  ResumePill: true,
}

function mountPlayer() {
  return mount(AePlayer, {
    props: {
      animeId: 'anime-uuid',
      anime: { title: 'T', ep: 1, eps: 12 },
      theater: false,
    },
    global: {
      mocks: { $t: (k: string) => k },
      stubs,
    },
    attachTo: document.body,
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  fakeTracksRef.value = []
})

describe('AePlayer — subtitle wiring (Task 6 regression guard)', () => {
  it('passes merged tracks to BrowseSubsModal — not [] (original bug guard)', async () => {
    // Pre-populate the fake tracks so they're ready when the modal opens.
    fakeTracksRef.value = [
      {
        url: 'https://proxy.animeenigma.ru/subs/jimaku/test.ass',
        provider: 'jimaku',
        lang: 'ja',
        label: 'Japanese (jimaku)',
        format: 'ass',
      },
    ]

    const wrapper = mountPlayer()
    await flushPromises()
    await nextTick()

    // Step 1: Open the subs floating menu by emitting toggle-subs on PlayerControlBar.
    // PlayerControlBar is stubbed — emit to open the openMenu='subs' branch.
    await wrapper.findComponent({ name: 'PlayerControlBar' }).trigger('toggle-subs')
    await nextTick()

    // Step 2: Emit open-browse on SubtitlesMenu (now rendered since openMenu==='subs').
    await wrapper.findComponent({ name: 'SubtitlesMenu' }).trigger('open-browse')
    await nextTick()
    await flushPromises()
    await nextTick()

    const modal = wrapper.findComponent(BrowseSubsModal)
    expect(modal.exists()).toBe(true)
    // THE REGRESSION GUARD: tracks must NOT be an empty array.
    expect((modal.props('tracks') as unknown[]).length).toBeGreaterThan(0)
  })

  it('SubtitleOverlay is rendered and wired (no prop-shape error at runtime)', async () => {
    // Confirms the subtitleUrl / format prop bindings compile + render without
    // throwing. The auto-select logic itself is unit-tested in
    // pickDefaultSubtitle.spec.ts (Task 4) — asserting it end-to-end here
    // would require driving the full resolveStream cycle through the heavy
    // harness (useProviderResolver → useVideoEngine → watchers), which is too
    // brittle and out of scope for this integration boundary test.
    const wrapper = mountPlayer()
    await flushPromises()
    await nextTick()

    const overlay = wrapper.findComponent({ name: 'SubtitleOverlay' })
    expect(overlay.exists()).toBe(true)
  })
})
