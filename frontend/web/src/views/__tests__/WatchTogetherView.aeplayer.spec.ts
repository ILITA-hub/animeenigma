/**
 * WatchTogetherView — aePlayer-only player mounting.
 *
 * WatchTogether is aePlayer-only (Kodik retired from WT 2026-06-19 — aePlayer
 * plays Kodik content internally). The view mounts the AePlayer SFC (stubbed)
 * with `:room` bound for ANY loaded room, including in-flight LEGACY rooms whose
 * stored `player` is a retired kind (e.g. 'kodik') — those upgrade gracefully.
 * The anime metadata AePlayer requires (`{ title, ep, eps, still }`) is sourced
 * from a `useAnime().fetchAnime` call wired into the view's bootstrap.
 *
 * Mocking approach mirrors the existing view-test conventions: every heavy
 * dependency stubbed, async player component replaced with a trivial stub, the
 * room composable + REST snapshot + anime fetch mocked so the live-room branch
 * renders deterministically.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import en from '@/locales/en.json'

// --------------------------------------------------------------------------
// Mutable room ref the composable mock returns — tests set `.player` before
// mounting to drive which branch renders.
// --------------------------------------------------------------------------
const roomRef = ref<Record<string, unknown> | null>(null)

const connect = vi.fn().mockResolvedValue(undefined)
const disconnect = vi.fn()

vi.mock('@/composables/useWatchTogetherRoom', () => ({
  useWatchTogetherRoom: () => ({
    room: roomRef,
    members: ref([]),
    messages: ref([]),
    reactions: ref([]),
    connectionStatus: ref('open'),
    emitChangeEpisode: vi.fn(),
    emitChangePlayer: vi.fn(),
    emitChangeTranslation: vi.fn(),
    sendChat: vi.fn(),
    sendReaction: vi.fn(),
    onError: vi.fn(() => () => {}),
    onRoomClosed: vi.fn(() => () => {}),
    onAuthExpired: vi.fn(() => () => {}),
    connect,
    disconnect,
  }),
}))

// REST pre-fetch returns a snapshot carrying the room (with anime_id) so the
// bootstrap resolves into the live-room branch.
const getRoom = vi.fn()
vi.mock('@/api/watch-together', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/watch-together')>()
  return {
    ...actual,
    getRoom: (...args: unknown[]) => getRoom(...args),
  }
})

// useAnime().fetchAnime supplies the metadata AePlayer needs.
const fetchAnime = vi.fn()
vi.mock('@/composables/useAnime', () => ({
  useAnime: () => ({ fetchAnime }),
}))

vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAuthenticated: true,
    token: 'tok',
    wtGuestToken: null,
    ensureGuestToken: vi.fn().mockResolvedValue('guest'),
  }),
}))

import WatchTogetherView from '../WatchTogetherView.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en } })

function makeRouter() {
  return createRouter({
    history: createWebHistory(),
    routes: [
      { path: '/', component: { template: '<div/>' } },
      { path: '/watch/room/:roomId', component: WatchTogetherView },
      { path: '/anime/:id', component: { template: '<div/>' } },
      { path: '/auth', component: { template: '<div/>' } },
    ],
  })
}

// Player stub records its props via a data-testid + serialized props. A
// KodikPlayer stub is kept so the "no legacy player rendered" assertions are
// meaningful even though the view no longer imports it.
const playerStub = (name: string) => ({
  name,
  props: ['animeId', 'anime', 'theater', 'isHentai', 'initialEpisode', 'malId', 'room'],
  template: `<div :data-testid="'${name}'" :data-has-room="room != null ? '1' : '0'"></div>`,
})

const globalStubs = {
  AePlayer: playerStub('AePlayer'),
  KodikPlayer: playerStub('KodikPlayer'),
  RoomSidebar: { template: '<div/>' },
  ReactionBurstOverlay: { template: '<div/>' },
  SyncToastStack: { template: '<div/>' },
  ConnectionStatusOverlay: { template: '<div/>' },
}

async function mountView() {
  const router = makeRouter()
  await router.push('/watch/room/room-1')
  await router.isReady()
  const wrapper = mount(WatchTogetherView, {
    global: { plugins: [router, i18n], stubs: globalStubs },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('WatchTogetherView — aePlayer-only mounting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    roomRef.value = null
    getRoom.mockResolvedValue({
      room: { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'aeplayer' },
      members: [],
      messages: [],
    })
    fetchAnime.mockResolvedValue({
      id: 'anime-uuid-1',
      title: 'Cowboy Bebop',
      episodesAired: 5,
      totalEpisodes: 26,
      coverImage: 'https://img/cover.jpg',
      shikimoriId: '1',
      rawGenres: [{ name: 'Action' }],
    })
  })

  it('mounts AePlayer with :room bound when player is aeplayer', async () => {
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'aeplayer' }
    const wrapper = await mountView()

    const ae = wrapper.find('[data-testid="AePlayer"]')
    expect(ae.exists()).toBe(true)
    expect(ae.attributes('data-has-room')).toBe('1')
  })

  it('never renders the standalone Kodik player', async () => {
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'aeplayer' }
    const wrapper = await mountView()

    expect(wrapper.find('[data-testid="KodikPlayer"]').exists()).toBe(false)
  })

  it('passes the fetched anime metadata object to AePlayer', async () => {
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'aeplayer' }
    const wrapper = await mountView()

    const ae = wrapper.findComponent({ name: 'AePlayer' })
    expect(ae.exists()).toBe(true)
    const animeProp = ae.props('anime') as { title: string; ep: number; eps: number; still?: string }
    expect(animeProp.title).toBe('Cowboy Bebop')
    expect(animeProp.ep).toBe(5)
    expect(animeProp.eps).toBe(26)
    expect(animeProp.still).toBe('https://img/cover.jpg')
    expect(ae.props('malId')).toBe('1')
    expect(ae.props('isHentai')).toBe(false)
    expect(ae.props('animeId')).toBe('anime-uuid-1')
    expect(fetchAnime).toHaveBeenCalledWith('anime-uuid-1')
  })

  it('gracefully upgrades an in-flight LEGACY kodik room to AePlayer', async () => {
    getRoom.mockResolvedValue({
      room: { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'kodik' },
      members: [],
      messages: [],
    })
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'kodik' }
    const wrapper = await mountView()

    // The retired standalone Kodik player is never mounted; the room renders
    // AePlayer instead (which plays Kodik content internally).
    expect(wrapper.find('[data-testid="KodikPlayer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="AePlayer"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="AePlayer"]').attributes('data-has-room')).toBe('1')
  })
})
