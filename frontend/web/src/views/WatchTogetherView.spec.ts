/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.8.
 *
 * Vitest spec for WatchTogetherView.vue.
 *
 * The view is the glue between the composable (Plan 02.3), the API client
 * (Plan 02.1), the sidebar (Plan 02.6), and the 5 existing `<*Player>`
 * components (which accept-but-ignore the `:room` prop after Plan 02.7).
 *
 * The 13 tests below lock the wiring contract:
 *
 *   1. On mount, calls `getRoom(roomId)` exactly once
 *   2. On `getRoom` → RoomGoneError, renders `room_ended_title` text
 *   3. On `getRoom` success, calls `roomHandle.connect()` exactly once
 *   4. room.player === 'kodik' → mounts <KodikPlayer> and NOT the other 4
 *   5. room.player === 'animelib' → mounts <AnimeLibPlayer>
 *   6. room.player === 'ourenglish' → mounts <OurEnglishPlayer>
 *   7. room.player === 'hanime' → mounts <HanimePlayer>
 *   8. room.player === 'raw' → mounts <RawPlayer>
 *   9. RoomSidebar receives room / members / messages / sendChat /
 *      sendReaction / connectionStatus props from the composable
 *  10. CAPACITY_FULL error → renders capacity-full state
 *  11. AUTH_EXPIRED error → router.push to /auth with returnUrl preserved
 *  12. On unmount, calls roomHandle.disconnect()
 *  13. No font-bold / font-black / font-extrabold in rendered HTML
 *
 * Strategy:
 *   - Mock `@/api/watch-together` with a vi.fn() `getRoom` + a real
 *     `RoomGoneError` subclass (so `instanceof` still works in the SUT).
 *   - Mock `@/composables/useWatchTogetherRoom` to return a controllable
 *     `fakeHandle` object. We expose the registered `onError` handler so
 *     tests can drive the CAPACITY_FULL / AUTH_EXPIRED branches.
 *   - Stub `vue-router` so we can capture `router.push` calls and feed in
 *     deterministic route params.
 *   - Stub all 5 player components + RoomSidebar + ReactionBurstOverlay via
 *     `global.stubs`. defineAsyncComponent's lazy-imports will resolve to
 *     the stub names registered on the test wrapper.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

// ── Mocks (must be hoisted before SUT import) ─────────────────────────────

class FakeRoomGoneError extends Error {
  constructor() {
    super('room gone')
    this.name = 'RoomGoneError'
    Object.setPrototypeOf(this, FakeRoomGoneError.prototype)
  }
}

const getRoomMock = vi.fn()

vi.mock('@/api/watch-together', () => ({
  getRoom: (id: string) => getRoomMock(id),
  RoomGoneError: FakeRoomGoneError,
  // Re-export error codes used by the SUT's error branching.
  ERR_CAPACITY_FULL: 'CAPACITY_FULL',
  ERR_AUTH_EXPIRED: 'AUTH_EXPIRED',
}))

// `useWatchTogetherRoom(roomId)` returns a stubbed handle whose `connect`
// resolves immediately. We expose the onError handler the SUT registers so
// individual tests can fire CAPACITY_FULL / AUTH_EXPIRED.
let lastErrorHandler: ((e: { code: string; message?: string }) => void) | null = null
let lastRoomClosedHandler: (() => void) | null = null

interface FakeHandle {
  room: ReturnType<typeof ref>
  members: ReturnType<typeof ref>
  messages: ReturnType<typeof ref>
  reactions: ReturnType<typeof ref>
  connectionStatus: ReturnType<typeof ref>
  lastError: ReturnType<typeof ref>
  sendChat: ReturnType<typeof vi.fn>
  sendReaction: ReturnType<typeof vi.fn>
  connect: ReturnType<typeof vi.fn>
  disconnect: ReturnType<typeof vi.fn>
  onError: ReturnType<typeof vi.fn>
  onRoomClosed: ReturnType<typeof vi.fn>
}

function makeFakeHandle(player: 'kodik' | 'animelib' | 'ourenglish' | 'hanime' | 'raw' = 'kodik'): FakeHandle {
  const handle: FakeHandle = {
    room: ref({
      id: 'room-abc',
      created_at: 0,
      anime_id: 'anime-1',
      episode_id: 'ep-1',
      player,
      translation_id: 'tr-1',
      playback_state: 'paused',
      playback_time: 0,
      playback_time_updated_at: 0,
      host_user_id: 'host-uuid',
    }),
    members: ref([]),
    messages: ref([]),
    reactions: ref([]),
    connectionStatus: ref('open'),
    lastError: ref(null),
    sendChat: vi.fn(),
    sendReaction: vi.fn(),
    connect: vi.fn().mockResolvedValue(undefined),
    disconnect: vi.fn(),
    onError: vi.fn((h: (e: { code: string }) => void) => {
      lastErrorHandler = h
      return () => {
        lastErrorHandler = null
      }
    }),
    onRoomClosed: vi.fn((h: () => void) => {
      lastRoomClosedHandler = h
      return () => {
        lastRoomClosedHandler = null
      }
    }),
  }
  return handle
}

let currentHandle: FakeHandle = makeFakeHandle()

vi.mock('@/composables/useWatchTogetherRoom', () => ({
  useWatchTogetherRoom: vi.fn(() => currentHandle),
}))

// vue-router mock — capture router.push for the AUTH_EXPIRED test.
const pushMock = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { roomId: 'room-abc' }, fullPath: '/watch/room/room-abc' }),
  useRouter: () => ({ push: pushMock }),
}))

// vue-i18n stub — echo keys.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

// SUT — must be imported AFTER all vi.mock calls.
import WatchTogetherView from './WatchTogetherView.vue'

// ── Child component stubs ────────────────────────────────────────────────

const RoomSidebarStub = {
  name: 'RoomSidebar',
  props: ['room', 'members', 'messages', 'sendChat', 'sendReaction', 'connectionStatus'],
  template: '<div data-testid="room-sidebar-stub" />',
}
const ReactionBurstOverlayStub = {
  name: 'ReactionBurstOverlay',
  props: ['reactions'],
  template: '<div data-testid="reaction-burst-overlay-stub" />',
}
const KodikPlayerStub = {
  name: 'KodikPlayer',
  props: ['animeId', 'initialEpisode', 'room'],
  template: '<div data-testid="kodik-player-stub" />',
}
const AnimeLibPlayerStub = {
  name: 'AnimeLibPlayer',
  props: ['animeId', 'initialEpisode', 'room'],
  template: '<div data-testid="animelib-player-stub" />',
}
const OurEnglishPlayerStub = {
  name: 'OurEnglishPlayer',
  props: ['animeId', 'initialEpisode', 'room'],
  template: '<div data-testid="ourenglish-player-stub" />',
}
const HanimePlayerStub = {
  name: 'HanimePlayer',
  props: ['animeId', 'initialEpisode', 'room'],
  template: '<div data-testid="hanime-player-stub" />',
}
const RawPlayerStub = {
  name: 'RawPlayer',
  props: ['animeId', 'room'],
  template: '<div data-testid="raw-player-stub" />',
}

const globalStubs = {
  RoomSidebar: RoomSidebarStub,
  ReactionBurstOverlay: ReactionBurstOverlayStub,
  KodikPlayer: KodikPlayerStub,
  AnimeLibPlayer: AnimeLibPlayerStub,
  OurEnglishPlayer: OurEnglishPlayerStub,
  HanimePlayer: HanimePlayerStub,
  RawPlayer: RawPlayerStub,
}

function mountView() {
  return mount(WatchTogetherView, {
    global: { stubs: globalStubs },
  })
}

// ── Test scaffolding ─────────────────────────────────────────────────────

beforeEach(() => {
  getRoomMock.mockReset()
  pushMock.mockReset()
  lastErrorHandler = null
  lastRoomClosedHandler = null
  currentHandle = makeFakeHandle()
})

afterEach(() => {
  vi.clearAllMocks()
})

function makeSnapshot(player: 'kodik' | 'animelib' | 'ourenglish' | 'hanime' | 'raw' = 'kodik') {
  return {
    room: {
      id: 'room-abc',
      created_at: 0,
      anime_id: 'anime-1',
      episode_id: 'ep-1',
      player,
      translation_id: 'tr-1',
      playback_state: 'paused' as const,
      playback_time: 0,
      playback_time_updated_at: 0,
      host_user_id: 'host-uuid',
    },
    members: [],
    messages: [],
    protocol_version: '1.0',
  }
}

// ── Tests ────────────────────────────────────────────────────────────────

describe('WatchTogetherView', () => {
  it('Test 1: on mount, calls getRoom(roomId) exactly once', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(getRoomMock).toHaveBeenCalledTimes(1)
    expect(getRoomMock).toHaveBeenCalledWith('room-abc')
  })

  it('Test 2: on RoomGoneError, renders room_ended_title text', async () => {
    getRoomMock.mockRejectedValueOnce(new FakeRoomGoneError())
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('watch_together.room_ended_title')
    expect(wrapper.text()).toContain('watch_together.room_ended_back_button')
  })

  it('Test 3: on getRoom success, calls roomHandle.connect() exactly once', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(currentHandle.connect).toHaveBeenCalledTimes(1)
  })

  it('Test 4: room.player === "kodik" mounts KodikPlayer and NOT the other 4', async () => {
    currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(AnimeLibPlayerStub).exists()).toBe(false)
    expect(wrapper.findComponent(OurEnglishPlayerStub).exists()).toBe(false)
    expect(wrapper.findComponent(HanimePlayerStub).exists()).toBe(false)
    expect(wrapper.findComponent(RawPlayerStub).exists()).toBe(false)
  })

  it('Test 5: room.player === "animelib" mounts AnimeLibPlayer', async () => {
    currentHandle = makeFakeHandle('animelib')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('animelib'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(AnimeLibPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 6: room.player === "ourenglish" mounts OurEnglishPlayer', async () => {
    currentHandle = makeFakeHandle('ourenglish')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('ourenglish'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(OurEnglishPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 7: room.player === "hanime" mounts HanimePlayer', async () => {
    currentHandle = makeFakeHandle('hanime')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('hanime'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(HanimePlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 8: room.player === "raw" mounts RawPlayer', async () => {
    currentHandle = makeFakeHandle('raw')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('raw'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(RawPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 9: RoomSidebar receives room/members/messages/sendChat/sendReaction/connectionStatus from composable', async () => {
    currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const sidebar = wrapper.findComponent(RoomSidebarStub)
    expect(sidebar.exists()).toBe(true)
    expect(sidebar.props('room')).toBe(currentHandle.room.value)
    expect(sidebar.props('members')).toBe(currentHandle.members.value)
    expect(sidebar.props('messages')).toBe(currentHandle.messages.value)
    expect(sidebar.props('sendChat')).toBe(currentHandle.sendChat)
    expect(sidebar.props('sendReaction')).toBe(currentHandle.sendReaction)
    expect(sidebar.props('connectionStatus')).toBe(currentHandle.connectionStatus.value)
  })

  it('Test 10: on CAPACITY_FULL error received, renders capacity-full state', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    expect(typeof lastErrorHandler).toBe('function')
    lastErrorHandler?.({ code: 'CAPACITY_FULL' })
    await flushPromises()
    expect(wrapper.text()).toContain('watch_together.capacity_full_title')
  })

  it('Test 11: on AUTH_EXPIRED error, calls router.push to /auth with returnUrl preserved', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(typeof lastErrorHandler).toBe('function')
    lastErrorHandler?.({ code: 'AUTH_EXPIRED' })
    await flushPromises()
    expect(pushMock).toHaveBeenCalledTimes(1)
    const arg = pushMock.mock.calls[0][0]
    expect(arg).toMatchObject({ path: '/auth' })
    // Either `query.next` or `query.returnUrl` is acceptable; both encode
    // the resume URL. The view's implementation picks one. We assert that
    // SOME query field carries the room URL.
    const q = arg.query as Record<string, string> | undefined
    const flat = q ? Object.values(q).join('|') : ''
    expect(flat).toContain('/watch/room/room-abc')
  })

  it('Test 12: on unmount, calls roomHandle.disconnect()', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    wrapper.unmount()
    expect(currentHandle.disconnect).toHaveBeenCalledTimes(1)
  })

  it('Test 13: rendered HTML uses only font-medium / font-semibold weights', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
  })
})
