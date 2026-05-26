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
//
// vi.mock factories are HOISTED above every other statement in the file
// (including `let` / `const` declarations), so the FakeRoomGoneError class
// and the getRoom mock are defined INSIDE the factory and re-surfaced via
// `vi.hoisted` so the test bodies can still reference them. This is the
// canonical vitest pattern for "I want both a mock surface AND test-body
// access to it" — see vitest.dev/api/vi.html#vi-hoisted.

const { getRoomMock, FakeRoomGoneError } = vi.hoisted(() => {
  class FakeRoomGoneError extends Error {
    constructor() {
      super('room gone')
      this.name = 'RoomGoneError'
      Object.setPrototypeOf(this, FakeRoomGoneError.prototype)
    }
  }
  return {
    getRoomMock: vi.fn(),
    FakeRoomGoneError,
  }
})

vi.mock('@/api/watch-together', () => ({
  getRoom: (id: string) => getRoomMock(id),
  RoomGoneError: FakeRoomGoneError,
  // Re-export error codes used by the SUT's error branching. The Phase 04
  // (state-switching) Plan 04.4 sender-only error trio joins the original
  // CAPACITY_FULL + AUTH_EXPIRED codes from Phase 02.
  ERR_CAPACITY_FULL: 'CAPACITY_FULL',
  ERR_AUTH_EXPIRED: 'AUTH_EXPIRED',
  ERR_EPISODE_UNAVAILABLE: 'EPISODE_UNAVAILABLE',
  ERR_PLAYER_UNAVAILABLE: 'PLAYER_UNAVAILABLE',
  ERR_TRANSLATION_UNAVAILABLE: 'TRANSLATION_UNAVAILABLE',
}))

// `useWatchTogetherRoom(roomId)` returns a stubbed handle whose `connect`
// resolves immediately. We expose the onError handler the SUT registers so
// individual tests can fire CAPACITY_FULL / AUTH_EXPIRED.
let lastErrorHandler: ((e: { code: string; message?: string }) => void) | null = null
// Phase 05 Plan 05.5 — onRoomClosed is now actively driven by tests
// (mid-session room:closed → router.push + toast). Capturing the handler.
let lastRoomClosedHandler: (() => void) | null = null
// Phase 05 Plan 05.5 — onAuthExpired channel. The view subscribes to set
// errorState='auth-expired' instead of firing an immediate router.push.
let lastAuthExpiredHandler: (() => void) | null = null

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
  // Plan 03.5 Task 4 wired SyncToastStack into the live-room branch; the
  // stack subscribes to `onPlaybackEvent` on mount, so the fake handle MUST
  // provide it (even as a no-op) or the view crashes the moment any test
  // reaches the live-room render path.
  onPlaybackEvent: ReturnType<typeof vi.fn>
  // Plan 04.4 Task 2 — PlayerTabBar routes select-player events through
  // roomHandle.emitChangePlayer (NOT direct local-state mutation).
  emitChangePlayer: ReturnType<typeof vi.fn>
  // Phase 05 Plan 05.5 — terminal-auth sugar channel; view binds to this
  // instead of branching inside onError for ERR_AUTH_EXPIRED.
  onAuthExpired: ReturnType<typeof vi.fn>
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
    // No-op subscriber for SyncToastStack. Returns its own unsubscriber
    // (also a no-op) so the component's `onBeforeUnmount` cleanup runs
    // without crashing.
    onPlaybackEvent: vi.fn(() => () => {}),
    // Plan 04.4 — captured by the PlayerTabBar @select-player handler.
    emitChangePlayer: vi.fn(),
    // Plan 05.5 — capture the view's auth-expired handler so tests can
    // drive errorState='auth-expired' without round-tripping through the
    // generic onError dispatcher.
    onAuthExpired: vi.fn((h: () => void) => {
      lastAuthExpiredHandler = h
      return () => {
        lastAuthExpiredHandler = null
      }
    }),
  }
  return handle
}

// Hoisted state holder: vitest hoists vi.mock factories above module-scope
// `let` declarations, so we stash mutable state inside `vi.hoisted` so the
// `useWatchTogetherRoom` factory and the test bodies share one source.
// The handle is typed as `unknown` here (vi.hoisted runs before our
// FakeHandle type exists in the bundling order); test bodies cast to
// FakeHandle on consumption.
interface SharedState {
  currentHandle: FakeHandle | null
  pushMock: ReturnType<typeof vi.fn>
  // Plan 04.4 — toast.push spy (separate from router.push above so the
  // AUTH_EXPIRED router test and the state-error toast tests don't share
  // a spy and confuse each other's call-count assertions).
  toastPushMock: ReturnType<typeof vi.fn>
}
const sharedState = vi.hoisted(
  () =>
    ({
      currentHandle: null,
      pushMock: vi.fn(),
      toastPushMock: vi.fn(),
    }) as SharedState,
)

vi.mock('@/composables/useWatchTogetherRoom', () => ({
  useWatchTogetherRoom: vi.fn(() => sharedState.currentHandle),
}))

// Plan 04.4 — useToast() is consumed by the SUT to surface sender-only
// state-error codes as user-visible toasts. Mock to a stable spy.
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({
    push: sharedState.toastPushMock,
    dismiss: vi.fn(),
    toasts: { value: [] },
  }),
}))

// vue-router mock — capture router.push for the AUTH_EXPIRED test.
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { roomId: 'room-abc' }, fullPath: '/watch/room/room-abc' }),
  useRouter: () => ({ push: sharedState.pushMock }),
}))

// vue-i18n stub — echo keys. Use importOriginal to preserve createI18n
// (referenced transitively via @/i18n.ts → @/stores/auth.ts).
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}::${JSON.stringify(params)}` : key,
      locale: { value: 'en' },
    }),
  }
})

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
// Plan 04.4 Task 2 — PlayerTabBar stub. Exposes a synthetic `select-player`
// emitter via a button so tests can drive the @select-player handler.
const PlayerTabBarStub = {
  name: 'PlayerTabBar',
  props: ['activePlayer', 'disabled'],
  emits: ['select-player'],
  template:
    '<div data-testid="player-tab-bar-stub" :data-active="activePlayer">' +
    '<button data-testid="tabbar-emit-animelib" @click="$emit(\'select-player\', \'animelib\')">switch</button>' +
    '<button data-testid="tabbar-emit-same" @click="$emit(\'select-player\', activePlayer)">same</button>' +
    '</div>',
}

const globalStubs = {
  RoomSidebar: RoomSidebarStub,
  ReactionBurstOverlay: ReactionBurstOverlayStub,
  KodikPlayer: KodikPlayerStub,
  AnimeLibPlayer: AnimeLibPlayerStub,
  OurEnglishPlayer: OurEnglishPlayerStub,
  HanimePlayer: HanimePlayerStub,
  RawPlayer: RawPlayerStub,
  PlayerTabBar: PlayerTabBarStub,
}

function mountView() {
  return mount(WatchTogetherView, {
    global: { stubs: globalStubs },
  })
}

// ── Test scaffolding ─────────────────────────────────────────────────────

beforeEach(() => {
  getRoomMock.mockReset()
  sharedState.pushMock.mockReset()
  sharedState.toastPushMock.mockReset()
  lastErrorHandler = null
  lastRoomClosedHandler = null
  lastAuthExpiredHandler = null
  sharedState.currentHandle = makeFakeHandle()
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
    expect(sharedState.currentHandle!.connect).toHaveBeenCalledTimes(1)
  })

  it('Test 4: room.player === "kodik" mounts KodikPlayer and NOT the other 4', async () => {
    sharedState.currentHandle = makeFakeHandle('kodik')
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
    sharedState.currentHandle = makeFakeHandle('animelib')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('animelib'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(AnimeLibPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 6: room.player === "ourenglish" mounts OurEnglishPlayer', async () => {
    sharedState.currentHandle = makeFakeHandle('ourenglish')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('ourenglish'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(OurEnglishPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 7: room.player === "hanime" mounts HanimePlayer', async () => {
    sharedState.currentHandle = makeFakeHandle('hanime')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('hanime'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(HanimePlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 8: room.player === "raw" mounts RawPlayer', async () => {
    sharedState.currentHandle = makeFakeHandle('raw')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('raw'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(RawPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
  })

  it('Test 9: RoomSidebar receives room/members/messages/sendChat/sendReaction/connectionStatus from composable', async () => {
    sharedState.currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const sidebar = wrapper.findComponent(RoomSidebarStub)
    const h = sharedState.currentHandle!
    expect(sidebar.exists()).toBe(true)
    expect(sidebar.props('room')).toBe(h.room.value)
    expect(sidebar.props('members')).toBe(h.members.value)
    expect(sidebar.props('messages')).toBe(h.messages.value)
    expect(sidebar.props('sendChat')).toBe(h.sendChat)
    expect(sidebar.props('sendReaction')).toBe(h.sendReaction)
    expect(sidebar.props('connectionStatus')).toBe(h.connectionStatus.value)
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

  it('Test 11: on AUTH_EXPIRED via onAuthExpired channel, errorState becomes "auth-expired" (NO immediate router.push)', async () => {
    // Plan 05.5: terminal AUTH_EXPIRED no longer fires an immediate
    // router.push. Instead, the view sets errorState='auth-expired' to
    // render the blocking modal. The user clicks Login to navigate; if
    // they close the tab, they don't end up at an auth page they didn't
    // ask for.
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    expect(typeof lastAuthExpiredHandler).toBe('function')
    lastAuthExpiredHandler?.()
    await flushPromises()
    // No router.push during this tick — the modal is shown.
    expect(sharedState.pushMock).not.toHaveBeenCalled()
    // Modal title key is rendered (i18n stub echoes keys).
    expect(wrapper.text()).toContain('watch_together.auth_expired_modal_title')
    expect(wrapper.text()).toContain('watch_together.auth_expired_modal_body')
    expect(wrapper.text()).toContain('watch_together.auth_expired_modal_login_button')
  })

  it('Test 12: on unmount, calls roomHandle.disconnect()', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const h = sharedState.currentHandle!
    wrapper.unmount()
    expect(h.disconnect).toHaveBeenCalledTimes(1)
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

  // ── Plan 04.4 Task 2 — re-mount + tab routing + state-error toasts ───

  it('Test 14: Player re-mount — mutating room.player from kodik to animelib swaps the mounted player', async () => {
    sharedState.currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    // Pre-condition: Kodik mounted, others not.
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(true)
    expect(wrapper.findComponent(AnimeLibPlayerStub).exists()).toBe(false)
    // Drive the state change as the composable's auto-mutator would.
    const h = sharedState.currentHandle!
    ;(h.room.value as { player: string }).player = 'animelib'
    await flushPromises()
    expect(wrapper.findComponent(KodikPlayerStub).exists()).toBe(false)
    expect(wrapper.findComponent(AnimeLibPlayerStub).exists()).toBe(true)
  })

  it('Test 15: PlayerTabBar @select-player → roomHandle.emitChangePlayer (single call, right arg)', async () => {
    sharedState.currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const h = sharedState.currentHandle!
    expect(h.emitChangePlayer).not.toHaveBeenCalled()
    await wrapper.find('[data-testid="tabbar-emit-animelib"]').trigger('click')
    expect(h.emitChangePlayer).toHaveBeenCalledTimes(1)
    expect(h.emitChangePlayer).toHaveBeenCalledWith('animelib')
  })

  it('Test 16: PlayerTabBar @select-player with currently-active kind is a no-op (no emitChangePlayer call)', async () => {
    sharedState.currentHandle = makeFakeHandle('kodik')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    const h = sharedState.currentHandle!
    // Stub's "same" button emits select-player with the current activePlayer.
    await wrapper.find('[data-testid="tabbar-emit-same"]').trigger('click')
    expect(h.emitChangePlayer).not.toHaveBeenCalled()
  })

  it('Test 17: EPISODE_UNAVAILABLE error → toast.push with i18n key + "error" type', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(typeof lastErrorHandler).toBe('function')
    lastErrorHandler?.({ code: 'EPISODE_UNAVAILABLE' })
    await flushPromises()
    expect(sharedState.toastPushMock).toHaveBeenCalledTimes(1)
    expect(sharedState.toastPushMock).toHaveBeenCalledWith(
      'watch_together.state_change_episode_unavailable',
      'error',
    )
  })

  it('Test 18: PLAYER_UNAVAILABLE error → toast.push with i18n key + "error" type', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    lastErrorHandler?.({ code: 'PLAYER_UNAVAILABLE' })
    await flushPromises()
    expect(sharedState.toastPushMock).toHaveBeenCalledTimes(1)
    expect(sharedState.toastPushMock).toHaveBeenCalledWith(
      'watch_together.state_change_player_unavailable',
      'error',
    )
  })

  it('Test 19: TRANSLATION_UNAVAILABLE error → toast.push with i18n key + "error" type', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    lastErrorHandler?.({ code: 'TRANSLATION_UNAVAILABLE' })
    await flushPromises()
    expect(sharedState.toastPushMock).toHaveBeenCalledTimes(1)
    expect(sharedState.toastPushMock).toHaveBeenCalledWith(
      'watch_together.state_change_translation_unavailable',
      'error',
    )
  })

  // ── Phase 05 Plan 05.5 — polish branches (WT-POLISH-04..07) ──

  it('Test 20: capacity_full title key resolves to a localized string in both en + ru locales', async () => {
    // The i18n stub echoes keys, so we just assert the key is rendered.
    // Locale-specific copy ("(10/10)" suffix) is locked by the i18n
    // parity test in src/locales/__tests__/watch-together-keys.spec.ts.
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    lastErrorHandler?.({ code: 'CAPACITY_FULL' })
    await flushPromises()
    expect(wrapper.text()).toContain('watch_together.capacity_full_title')
    expect(wrapper.text()).toContain('watch_together.capacity_full_back_button')
  })

  it('Test 21: mid-session onRoomClosed → router.push to /anime/{id}/watch + toast', async () => {
    // WT-POLISH-05: the WS-side room:closed event (vs REST 410) should
    // redirect to the anime watch page with a toast, NOT show the empty
    // "room ended" page. This distinguishes "user typed stale URL" (REST
    // 410 → empty page) from "user was watching, room ended" (WS event
    // → redirect with toast).
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(typeof lastRoomClosedHandler).toBe('function')
    lastRoomClosedHandler?.()
    await flushPromises()
    expect(sharedState.toastPushMock).toHaveBeenCalledTimes(1)
    expect(sharedState.toastPushMock).toHaveBeenCalledWith(
      'watch_together.room_ended_redirect_toast',
      'info',
    )
    expect(sharedState.pushMock).toHaveBeenCalledTimes(1)
    expect(sharedState.pushMock).toHaveBeenCalledWith('/anime/anime-1/watch')
  })

  it('Test 22: onRoomClosed without animeId falls back to "/"', async () => {
    // Defensive: if the REST snapshot never resolved (e.g. WS-only delivery
    // of a snapshot that arrived faster than getRoom returned), the view
    // doesn't have an anime id. Falls back to home root.
    sharedState.currentHandle = makeFakeHandle('kodik')
    sharedState.currentHandle.room.value = null
    sessionStorage.removeItem('wt-last-anime-id-room-abc')
    // Throw on the REST pre-fetch so lastAnimeId never gets seeded.
    getRoomMock.mockReset()
    getRoomMock.mockRejectedValueOnce(new Error('network'))
    mountView()
    await flushPromises()
    // The catch branch sets errorState='gone' but doesn't redirect on REST
    // failure (Phase 2 behavior — preserved). Now drive the WS-side
    // onRoomClosed: with no anime id, it should fall back to '/'.
    lastRoomClosedHandler?.()
    await flushPromises()
    expect(sharedState.pushMock).toHaveBeenCalledWith('/')
  })

  it('Test 23: REST 410 initial bootstrap shows the "gone" empty state (no router.push)', async () => {
    // WT-POLISH-05: distinguish REST 410 from mid-session WS room:closed.
    // REST 410 keeps the friendlier empty page with a Back button — no
    // redirect.
    getRoomMock.mockRejectedValueOnce(new FakeRoomGoneError())
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('watch_together.room_ended_title')
    expect(sharedState.pushMock).not.toHaveBeenCalled()
    expect(sharedState.toastPushMock).not.toHaveBeenCalled()
  })

  it('Test 24: auth-expired modal renders title/body/login button with dialog role + aria-modal', async () => {
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    lastAuthExpiredHandler?.()
    await flushPromises()
    const dialog = wrapper.find('[role="dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(wrapper.text()).toContain('watch_together.auth_expired_modal_title')
    expect(wrapper.text()).toContain('watch_together.auth_expired_modal_body')
    const button = wrapper.find('button[data-testid="wt-auth-expired-login"]')
    expect(button.exists()).toBe(true)
    expect(button.text()).toContain('watch_together.auth_expired_modal_login_button')
  })

  it('Test 25: clicking Login in auth-expired modal pushes to /auth?next=… and writes returnUrl', async () => {
    const setItemSpy = vi.spyOn(Storage.prototype, 'setItem')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    const wrapper = mountView()
    await flushPromises()
    lastAuthExpiredHandler?.()
    await flushPromises()
    const button = wrapper.find('button[data-testid="wt-auth-expired-login"]')
    expect(button.exists()).toBe(true)
    await button.trigger('click')
    expect(sharedState.pushMock).toHaveBeenCalledTimes(1)
    const arg = sharedState.pushMock.mock.calls[0][0] as {
      path: string
      query: Record<string, string>
    }
    expect(arg.path).toBe('/auth')
    expect(arg.query.next).toBe('/watch/room/room-abc')
    // Belt-and-suspenders: returnUrl persisted to sessionStorage too.
    expect(setItemSpy).toHaveBeenCalledWith('returnUrl', '/watch/room/room-abc')
    setItemSpy.mockRestore()
  })

  it('Test 26: bootstrap success persists lastAnimeId to sessionStorage (survives WS-only room:closed)', async () => {
    // WT-POLISH-05 underpinning: the WS room:closed handler needs an
    // anime_id even when the composable's room ref has already been
    // cleared. The view caches it on REST bootstrap success.
    const setItemSpy = vi.spyOn(Storage.prototype, 'setItem')
    getRoomMock.mockResolvedValueOnce(makeSnapshot('kodik'))
    mountView()
    await flushPromises()
    expect(setItemSpy).toHaveBeenCalledWith('wt-last-anime-id-room-abc', 'anime-1')
    setItemSpy.mockRestore()
  })
})
