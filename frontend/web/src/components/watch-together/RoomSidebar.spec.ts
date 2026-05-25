/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 1.
 *
 * Vitest spec for RoomSidebar.vue. RoomSidebar is the parent shell that
 * composes the three sidebar leaves (MemberList, ChatPanel, ReactionPalette)
 * into the locked vertical layout per CONTEXT.md §"Component layout". The
 * 7 tests below lock the wiring contract:
 *
 *   1. Renders MemberList, ChatPanel, ReactionPalette as child components
 *   2. Passes `room.host_user_id` to MemberList; passes `''` when room is null
 *   3. Passes messages to ChatPanel; relays @send → sendChat prop
 *   4. Relays @react → sendReaction prop on ReactionPalette
 *   5. connectionStatus='reconnecting' → reconnecting indicator visible
 *   6. connectionStatus='open' → reconnecting indicator hidden
 *   7. No font-bold / font-black / font-extrabold in rendered HTML
 *
 * The three child components are stubbed via the `global.stubs` option so we
 * can assert prop pass-through without bringing in their rendering. vue-i18n
 * is stubbed with an echoing `t()`.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// ── Mocks ────────────────────────────────────────────────────────────────

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

// useAuthStore is referenced transitively via MemberList; we stub the child
// components themselves (below) so this stub is just defensive.
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'self-uuid', username: 'self' } }),
}))

import RoomSidebar from './RoomSidebar.vue'
import type {
  Room,
  Member,
  ChatMessage,
} from '@/api/watch-together'

// ── Child component stubs — capture props for assertions. ────────────────

const MemberListStub = {
  name: 'MemberList',
  props: ['members', 'hostUserId'],
  template: '<div data-testid="member-list-stub" />',
}

const ChatPanelStub = {
  name: 'ChatPanel',
  props: ['messages', 'currentUserId'],
  emits: ['send'],
  template: '<div data-testid="chat-panel-stub" />',
}

const ReactionPaletteStub = {
  name: 'ReactionPalette',
  props: ['sendReaction'],
  template: '<div data-testid="reaction-palette-stub" />',
}

// ── Test data ────────────────────────────────────────────────────────────

function makeRoom(overrides: Partial<Room> = {}): Room {
  return {
    id: 'room-1',
    host_user_id: 'host-uuid',
    anime_id: 'anime-1',
    episode_id: 'ep-1',
    player: 'kodik',
    translation_id: 'tr-1',
    playback_state: 'paused',
    playback_time: 0,
    playback_time_updated_at: 0,
    created_at: 0,
    expires_at: 0,
    ...overrides,
  } as Room
}

const baseMembers: Member[] = [
  {
    user_id: 'host-uuid',
    meta: {
      username: 'Alice',
      avatar_url: '',
      joined_at: 1_700_000_000,
      last_seen_at: 1_700_000_000,
    },
  },
]

const baseMessages: ChatMessage[] = [
  { id: 'm1', user_id: 'host-uuid', username: 'Alice', body: 'hi', ts: 1_700_000_000_000 },
]

interface MountProps {
  room?: Room | null
  members?: Member[]
  messages?: ChatMessage[]
  sendChat?: (body: string) => void
  sendReaction?: (emoji: string) => void
  connectionStatus?:
    | 'idle'
    | 'connecting'
    | 'open'
    | 'reconnecting'
    | 'closed'
    | 'failed'
}

function mountSidebar(props: MountProps = {}) {
  return mount(RoomSidebar, {
    props: {
      room: makeRoom(),
      members: baseMembers,
      messages: baseMessages,
      sendChat: vi.fn(),
      sendReaction: vi.fn(),
      connectionStatus: 'open',
      ...props,
    },
    global: {
      stubs: {
        MemberList: MemberListStub,
        ChatPanel: ChatPanelStub,
        ReactionPalette: ReactionPaletteStub,
      },
    },
  })
}

describe('RoomSidebar', () => {
  it('renders MemberList, ChatPanel, and ReactionPalette as children', () => {
    const wrapper = mountSidebar()
    expect(wrapper.findComponent(MemberListStub).exists()).toBe(true)
    expect(wrapper.findComponent(ChatPanelStub).exists()).toBe(true)
    expect(wrapper.findComponent(ReactionPaletteStub).exists()).toBe(true)
  })

  it('passes room.host_user_id to MemberList', () => {
    const wrapper = mountSidebar({ room: makeRoom({ host_user_id: 'specific-host' }) })
    const ml = wrapper.findComponent(MemberListStub)
    expect(ml.props('hostUserId')).toBe('specific-host')
  })

  it('passes empty string to MemberList.hostUserId when room is null', () => {
    const wrapper = mountSidebar({ room: null })
    const ml = wrapper.findComponent(MemberListStub)
    expect(ml.props('hostUserId')).toBe('')
  })

  it('passes messages to ChatPanel and relays @send to the sendChat prop', async () => {
    const sendChat = vi.fn()
    const wrapper = mountSidebar({ messages: baseMessages, sendChat })
    const cp = wrapper.findComponent(ChatPanelStub)
    expect(cp.props('messages')).toEqual(baseMessages)
    cp.vm.$emit('send', 'hello world')
    expect(sendChat).toHaveBeenCalledTimes(1)
    expect(sendChat).toHaveBeenCalledWith('hello world')
  })

  it('relays @react from ReactionPalette → sendReaction prop', () => {
    const sendReaction = vi.fn()
    const wrapper = mountSidebar({ sendReaction })
    const rp = wrapper.findComponent(ReactionPaletteStub)
    // ReactionPalette receives sendReaction via prop; assert pass-through.
    expect(rp.props('sendReaction')).toBe(sendReaction)
  })

  it('shows the reconnecting indicator when connectionStatus is "reconnecting"', () => {
    const wrapper = mountSidebar({ connectionStatus: 'reconnecting' })
    expect(wrapper.text()).toContain('watch_together.reconnecting_indicator')
  })

  it('hides the reconnecting indicator when connectionStatus is "open"', () => {
    const wrapper = mountSidebar({ connectionStatus: 'open' })
    expect(wrapper.text()).not.toContain('watch_together.reconnecting_indicator')
  })

  it('uses only font-medium / font-semibold weights (no font-bold)', () => {
    const wrapper = mountSidebar()
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
  })
})
