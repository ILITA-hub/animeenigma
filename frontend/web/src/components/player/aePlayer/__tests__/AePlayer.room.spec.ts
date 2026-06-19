import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { Ref } from 'vue'
import type { Room } from '@/types/watch-together'
import { comboToToken } from '@/composables/aePlayer/comboMapping'

// AePlayer exposes its live `state.combo` via defineExpose({ __combo }) so the
// test can read the pinned/applied combo without mocking usePlayerState (which
// hands every caller an independent instance). The real usePlayerState runs.

// ─── Heavy network/store composables — stubbed so the component mounts ─────────
vi.mock('@/composables/aePlayer/useProviderHealth', () => ({
  useProviderHealth: () => ({ rows: ref([]), start: vi.fn() }),
}))
vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({ capMap: ref(new Map()), rankedIds: ref([]) }),
}))
const listEpisodes = vi.fn().mockResolvedValue([])
const listTeams = vi.fn().mockResolvedValue([])
const resolveStream = vi.fn().mockResolvedValue({ type: 'hls', url: '', servers: [] })
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({ listEpisodes, listTeams, resolveStream }),
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

// useWatchTogetherLaunch (in-player WT button) calls useI18n() at setup; the
// player otherwise uses only template $t. Stub the module so the mount doesn't
// require an installed i18n plugin. (auth + api/client are already mocked, so
// this never reaches src/i18n.ts / createI18n.)
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }),
}))

// Stores — return minimal shapes the component reads.
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: false, user: null }),
}))
vi.mock('@/stores/viewerContext', () => ({
  useViewerContextStore: () => ({ whenLoaded: vi.fn().mockResolvedValue(null) }),
}))

// API clients touched on mount (ae availability probe, progress fetch).
vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn().mockResolvedValue({ data: { data: null } }) },
  aeApi: { getEpisodes: vi.fn().mockResolvedValue({ data: { data: { available: false, episodes: [] } } }) },
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [] } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [] } } }),
    getStream: vi.fn().mockResolvedValue({ data: { data: { stream: { sources: [] } } } }),
  },
}))

// Telemetry — no-op (avoids beacon side effects).
vi.mock('@/utils/playerTelemetry', () => ({ recordPlayerEvent: vi.fn() }))

import AePlayer from '../AePlayer.vue'

// ─── Fake WT room handle ──────────────────────────────────────────────────────
interface FakeRoom {
  room: Ref<Room | null>
  emitPlay: ReturnType<typeof vi.fn>
  emitPause: ReturnType<typeof vi.fn>
  emitSeek: ReturnType<typeof vi.fn>
  emitTimeTick: ReturnType<typeof vi.fn>
  emitChangeEpisode: ReturnType<typeof vi.fn>
  emitChangePlayer: ReturnType<typeof vi.fn>
  emitChangeTranslation: ReturnType<typeof vi.fn>
  onPlaybackEvent: ReturnType<typeof vi.fn>
  onCorrection: ReturnType<typeof vi.fn>
}

function makeRoom(over: Partial<Room> = {}): FakeRoom {
  const room = ref<Room | null>({
    id: 'r1',
    created_at: 0,
    anime_id: 'anime-uuid',
    episode_id: '',
    player: 'aeplayer',
    translation_id: '',
    playback_state: 'paused',
    playback_time: 0,
    playback_time_updated_at: 0,
    host_user_id: 'u1',
    ...over,
  })
  return {
    room,
    emitPlay: vi.fn(),
    emitPause: vi.fn(),
    emitSeek: vi.fn(),
    emitTimeTick: vi.fn(),
    emitChangeEpisode: vi.fn(),
    emitChangePlayer: vi.fn(),
    emitChangeTranslation: vi.fn(),
    // Real bridge subscribes to these; return an unsubscribe fn.
    onPlaybackEvent: vi.fn().mockReturnValue(() => {}),
    onCorrection: vi.fn().mockReturnValue(() => {}),
  }
}

const stubs = {
  PlayerControlBar: true,
  SourcePanel: true,
  EpisodesPanel: true,
  PlaybackSettingsMenu: true,
  SubtitlesMenu: true,
  BrowseSubsModal: true,
  BigPlayButton: true,
  BufferingOverlay: true,
  DebugHud: true,
  SkipIntroChip: true,
  NextEpisodeCard: true,
  WatchTogetherButton: true,
  SubtitleOverlay: true,
  ResumePill: true,
}

function mountPlayer(roomHandle: FakeRoom | null) {
  return mount(AePlayer, {
    props: {
      animeId: 'anime-uuid',
      anime: { title: 'T', ep: 1, eps: 12 },
      theater: false,
      // FakeRoom is structurally compatible with the subset of the handle that
      // AePlayer + usePlayerSyncBridge consume.
      room: roomHandle as unknown as undefined,
    },
    global: {
      mocks: { $t: (k: string) => k },
      stubs,
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('AePlayer — WT room sync (Sub-Part A: HTML5 bridge)', () => {
  it('emits play to the room when the native <video> play event fires', async () => {
    const room = makeRoom()
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()

    const videoEl = wrapper.find('video').element as HTMLVideoElement
    // The bridge reads currentTime; jsdom defaults to 0.
    videoEl.dispatchEvent(new Event('play'))
    await nextTick()

    expect(room.emitPlay).toHaveBeenCalledTimes(1)
  })

  it('does NOT emit when no room prop is provided', async () => {
    const room = makeRoom()
    // Mount WITHOUT a room; reuse the same spies to prove they are never called.
    const wrapper = mountPlayer(null)
    await flushPromises()
    await nextTick()

    const videoEl = wrapper.find('video').element as HTMLVideoElement
    videoEl.dispatchEvent(new Event('play'))
    await nextTick()

    expect(room.emitPlay).not.toHaveBeenCalled()
  })
})

// Read the live combo exposed by AePlayer via defineExpose({ __combo }).
// test-utils unwraps the exposed ref on `vm`, so `__combo` is the Combo object
// itself (not a { value } wrapper).
function readCombo(wrapper: ReturnType<typeof mountPlayer>): import('@/types/aePlayer').Combo {
  const exposed = (wrapper.vm as unknown as { __combo: unknown }).__combo
  const maybeRef = exposed as { value?: import('@/types/aePlayer').Combo }
  return (maybeRef && 'value' in maybeRef && maybeRef.value
    ? maybeRef.value
    : exposed) as import('@/types/aePlayer').Combo
}

describe('AePlayer — WT room sync (Sub-Part B: pin to room combo)', () => {
  it('applies the room translation_id combo on mount and keeps it (no auto-override)', async () => {
    const token = comboToToken({
      provider: 'allanime',
      audio: 'sub',
      lang: 'en',
      team: null,
      server: 'wixmp',
    })
    const room = makeRoom({ player: 'aeplayer', translation_id: token })
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()
    await flushPromises()

    const combo = readCombo(wrapper)
    expect(combo.provider).toBe('allanime')
    expect(combo.audio).toBe('sub')
    expect(combo.lang).toBe('en')
    expect(combo.server).toBe('wixmp')
  })

  it('re-applies the combo when the room translation_id changes (remote source switch)', async () => {
    const first = comboToToken({
      provider: 'allanime', audio: 'sub', lang: 'en', team: null, server: 'wixmp',
    })
    const room = makeRoom({ player: 'aeplayer', translation_id: first })
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()

    expect(readCombo(wrapper).provider).toBe('allanime')

    // Remote member switches source → room.translation_id mutates.
    room.room.value!.translation_id = comboToToken({
      provider: 'miruro', audio: 'dub', lang: 'en', team: null, server: 'kiwi',
    })
    await nextTick()
    await flushPromises()

    const combo = readCombo(wrapper)
    expect(combo.provider).toBe('miruro')
    expect(combo.audio).toBe('dub')
    expect(combo.server).toBe('kiwi')
  })
})

// Invoke the exposed __setProvider seam to simulate a genuine local user pick.
function setProvider(wrapper: ReturnType<typeof mountPlayer>, id: string, server: string) {
  ;(wrapper.vm as unknown as { __setProvider: (id: string, server: string) => void }).__setProvider(
    id,
    server,
  )
}

describe('AePlayer — WT room sync (Sub-Part C: broadcast combo + episode)', () => {
  it('does NOT echo the combo back to the room when applied FROM the room (join)', async () => {
    const token = comboToToken({
      provider: 'allanime', audio: 'sub', lang: 'en', team: null, server: 'wixmp',
    })
    const room = makeRoom({ player: 'aeplayer', translation_id: token })
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()
    await flushPromises()

    // The combo is now 'allanime' (applied from the room) — but the player must
    // not have broadcast it back (no echo on join).
    expect(readCombo(wrapper).provider).toBe('allanime')
    expect(room.emitChangeTranslation).not.toHaveBeenCalled()

    // A remote source switch also must not echo.
    room.room.value!.translation_id = comboToToken({
      provider: 'miruro', audio: 'sub', lang: 'en', team: null, server: 'kiwi',
    })
    await nextTick()
    await flushPromises()
    expect(room.emitChangeTranslation).not.toHaveBeenCalled()
  })

  it('broadcasts a genuine LOCAL combo change to the room', async () => {
    const token = comboToToken({
      provider: 'allanime', audio: 'sub', lang: 'en', team: null, server: 'wixmp',
    })
    const room = makeRoom({ player: 'aeplayer', translation_id: token })
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()
    await flushPromises()
    room.emitChangeTranslation.mockClear()

    // User picks a different provider locally.
    setProvider(wrapper, 'gogoanime', '')
    await nextTick()
    await flushPromises()

    expect(room.emitChangeTranslation).toHaveBeenCalledTimes(1)
    const sent = room.emitChangeTranslation.mock.calls[0][0] as string
    expect(comboToToken(readCombo(wrapper) as never)).toBe(sent)
    const parsed = JSON.parse(sent) as { provider: string }
    expect(parsed.provider).toBe('gogoanime')
  })

  it('emits a local episode change to the room and applies a remote episode change', async () => {
    const token = comboToToken({
      provider: 'allanime', audio: 'sub', lang: 'en', team: null, server: 'wixmp',
    })
    const room = makeRoom({ player: 'aeplayer', translation_id: token, episode_id: '' })
    const wrapper = mountPlayer(room)
    await flushPromises()
    await nextTick()
    await flushPromises()

    // Local episode pick → emitChangeEpisode with the episode NUMBER as string.
    const vm = wrapper.vm as unknown as {
      onSelectEpisode: (ep: { key: number; label: number; number: number }) => void
    }
    vm.onSelectEpisode({ key: 4, label: 4, number: 4 })
    await nextTick()
    await flushPromises()
    expect(room.emitChangeEpisode).toHaveBeenCalledWith('4')

    // Remote episode change → room.episode_id mutates; the player adopts it
    // WITHOUT re-emitting (no echo).
    room.emitChangeEpisode.mockClear()
    room.room.value!.episode_id = '7'
    await nextTick()
    await flushPromises()
    expect(room.emitChangeEpisode).not.toHaveBeenCalled()
  })
})
