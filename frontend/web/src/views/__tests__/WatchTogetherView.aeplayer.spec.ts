/**
 * Plan A — AePlayer branch in WatchTogetherView.
 *
 * Verifies that when the room's `player` is `'aeplayer'`, the view mounts
 * the AePlayer SFC (stubbed) with `:room` bound, and does NOT render any
 * legacy player. The anime metadata object AePlayer requires
 * (`{ title, ep, eps, still }`) is sourced from a `useAnime().fetchAnime`
 * call wired into the view's bootstrap.
 *
 * Mocking approach mirrors the existing view-test conventions
 * (Profile.showcase.spec.ts): every heavy dependency stubbed, async player
 * components replaced with trivial stubs, the room composable + REST
 * snapshot + anime fetch mocked so the live-room branch renders
 * deterministically.
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

const emitChangePlayer = vi.fn()
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
    emitChangePlayer,
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

// Stubs for every player + shell child so the mount is light. Each player
// stub records its props via a data-testid + serialized props.
const playerStub = (name: string) => ({
  name,
  props: ['animeId', 'anime', 'theater', 'isHentai', 'initialEpisode', 'malId', 'room'],
  template: `<div :data-testid="'${name}'" :data-has-room="room != null ? '1' : '0'"></div>`,
})

const globalStubs = {
  KodikPlayer: playerStub('KodikPlayer'),
  KodikAdFreePlayer: playerStub('KodikAdFreePlayer'),
  AnimeLibPlayer: playerStub('AnimeLibPlayer'),
  OurEnglishPlayer: playerStub('OurEnglishPlayer'),
  HanimePlayer: playerStub('HanimePlayer'),
  RawPlayer: playerStub('RawPlayer'),
  AePlayer: playerStub('AePlayer'),
  RoomSidebar: { template: '<div/>' },
  ReactionBurstOverlay: { template: '<div/>' },
  SyncToastStack: { template: '<div/>' },
  ConnectionStatusOverlay: { template: '<div/>' },
  PlayerTabBar: { template: '<div/>' },
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

describe('WatchTogetherView — aeplayer branch', () => {
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

  it('does NOT render any legacy player for an aeplayer room', async () => {
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'aeplayer' }
    const wrapper = await mountView()

    expect(wrapper.find('[data-testid="KodikPlayer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="OurEnglishPlayer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="RawPlayer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="HanimePlayer"]').exists()).toBe(false)
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

  it('still mounts a legacy player (kodik) when player is not aeplayer', async () => {
    getRoom.mockResolvedValue({
      room: { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'kodik' },
      members: [],
      messages: [],
    })
    roomRef.value = { id: 'room-1', anime_id: 'anime-uuid-1', episode_id: '3', player: 'kodik' }
    const wrapper = await mountView()

    expect(wrapper.find('[data-testid="KodikPlayer"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="AePlayer"]').exists()).toBe(false)
  })
})
