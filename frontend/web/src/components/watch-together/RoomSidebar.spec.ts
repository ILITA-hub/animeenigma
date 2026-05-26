/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 1
 *                           + Phase 05 (polish) Plan 05.4 Task 2 (WT-POLISH-03).
 *
 * Vitest spec for RoomSidebar.vue. RoomSidebar is the parent shell that
 * composes the three sidebar leaves (MemberList, ChatPanel, ReactionPalette)
 * into the locked vertical layout per CONTEXT.md §"Component layout". As of
 * Plan 05.4 the component renders BOTH:
 *   - Desktop (>= lg): the original right-rail layout (8 Phase 2 tests)
 *   - Mobile (< lg):   a bottom-anchored 2-tab sheet (Chat | Reactions) with
 *                      collapsed / expanded states + drag gestures.
 *
 * Tailwind's `lg:hidden` / `hidden lg:flex` modifiers gate the two branches —
 * both are rendered in the DOM tree and CSS controls visibility. The spec
 * asserts presence of the gating classes rather than simulating viewport.
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

  // ── Phase 05 (polish) Plan 05.4 — WT-POLISH-03 mobile bottom-sheet ──────
  //
  // The component now renders TWO siblings: the desktop right-rail aside and
  // a mobile bottom-sheet aside. Tailwind `hidden lg:flex` / `lg:hidden`
  // controls which the user sees; the DOM contains both. We assert on
  // class gating + tab-bar behavior + sheet collapsed/expanded states.

  describe('mobile bottom-sheet (Plan 05.4 WT-POLISH-03)', () => {
    function mobileSheet(wrapper: ReturnType<typeof mountSidebar>) {
      // The mobile sheet is the <aside> with the `lg:hidden` class.
      const sheets = wrapper.findAll('aside').filter((a) =>
        a.classes().includes('lg:hidden'),
      )
      return sheets[0]
    }

    it('renders a fixed bottom-anchored panel gated by lg:hidden', () => {
      const wrapper = mountSidebar()
      const sheet = mobileSheet(wrapper)
      expect(sheet).toBeDefined()
      const cls = sheet.classes()
      expect(cls).toContain('lg:hidden')
      expect(cls).toContain('fixed')
      expect(cls).toContain('bottom-0')
    })

    it('desktop right-rail branch is gated by hidden + lg:flex', () => {
      const wrapper = mountSidebar()
      const desktopAside = wrapper
        .findAll('aside')
        .filter((a) => a.classes().includes('lg:flex'))[0]
      expect(desktopAside).toBeDefined()
      expect(desktopAside.classes()).toContain('hidden')
    })

    it('renders exactly 2 tab buttons (Chat | Reactions) in the tab bar', () => {
      const wrapper = mountSidebar()
      const tabs = wrapper.findAll('[role="tab"]')
      expect(tabs.length).toBe(2)
      const texts = tabs.map((t) => t.text())
      expect(texts).toContain('watch_together.bottom_sheet_tab_chat')
      expect(texts).toContain('watch_together.bottom_sheet_tab_reactions')
    })

    it('Chat tab is active by default', () => {
      const wrapper = mountSidebar()
      const tabs = wrapper.findAll('[role="tab"]')
      const chatTab = tabs.find((t) =>
        t.text().includes('bottom_sheet_tab_chat'),
      )!
      const reactionsTab = tabs.find((t) =>
        t.text().includes('bottom_sheet_tab_reactions'),
      )!
      expect(chatTab.attributes('aria-selected')).toBe('true')
      expect(reactionsTab.attributes('aria-selected')).toBe('false')
    })

    it('sheet starts collapsed (height = 80px)', () => {
      const wrapper = mountSidebar()
      const sheet = mobileSheet(wrapper)
      expect(sheet.attributes('style') ?? '').toContain('height: 80px')
      expect(sheet.attributes('aria-expanded')).toBe('false')
    })

    it('clicking Reactions tab switches active and expands sheet', async () => {
      const wrapper = mountSidebar()
      const tabs = wrapper.findAll('[role="tab"]')
      const reactionsTab = tabs.find((t) =>
        t.text().includes('bottom_sheet_tab_reactions'),
      )!
      await reactionsTab.trigger('click')
      const tabsAfter = wrapper.findAll('[role="tab"]')
      const chatAfter = tabsAfter.find((t) =>
        t.text().includes('bottom_sheet_tab_chat'),
      )!
      const reactionsAfter = tabsAfter.find((t) =>
        t.text().includes('bottom_sheet_tab_reactions'),
      )!
      expect(reactionsAfter.attributes('aria-selected')).toBe('true')
      expect(chatAfter.attributes('aria-selected')).toBe('false')
      const sheet = mobileSheet(wrapper)
      expect(sheet.attributes('aria-expanded')).toBe('true')
      expect(sheet.attributes('style') ?? '').toContain('height: 60vh')
    })

    it('clicking the active tab again collapses the sheet (toggle)', async () => {
      const wrapper = mountSidebar()
      const tabs = wrapper.findAll('[role="tab"]')
      const chatTab = tabs.find((t) =>
        t.text().includes('bottom_sheet_tab_chat'),
      )!
      // Chat is active by default, but collapsed. First click expands.
      await chatTab.trigger('click')
      let sheet = mobileSheet(wrapper)
      expect(sheet.attributes('aria-expanded')).toBe('true')
      // Second click on the active tab collapses.
      await chatTab.trigger('click')
      sheet = mobileSheet(wrapper)
      expect(sheet.attributes('aria-expanded')).toBe('false')
    })

    it('member count strip shows in tab bar (members.length / 10)', () => {
      const members: Member[] = [
        { user_id: 'a', meta: { username: 'A', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
        { user_id: 'b', meta: { username: 'B', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
        { user_id: 'c', meta: { username: 'C', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
      ]
      const wrapper = mountSidebar({ members })
      const sheet = mobileSheet(wrapper)
      expect(sheet.text()).toContain('3/10')
    })

    it('mobile sheet shows reconnecting banner when connectionStatus is reconnecting', () => {
      const wrapper = mountSidebar({ connectionStatus: 'reconnecting' })
      const sheet = mobileSheet(wrapper)
      expect(sheet.text()).toContain('watch_together.reconnecting_indicator')
    })

    it('mobile sheet hides reconnecting banner when connectionStatus is open', () => {
      const wrapper = mountSidebar({ connectionStatus: 'open' })
      const sheet = mobileSheet(wrapper)
      expect(sheet.text()).not.toContain('watch_together.reconnecting_indicator')
    })

    it('drag-up gesture (>50px) expands the sheet', async () => {
      const wrapper = mountSidebar()
      const sheet = mobileSheet(wrapper)
      await sheet.trigger('touchstart', {
        touches: [{ clientY: 300 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchmove', {
        touches: [{ clientY: 240 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchend')
      const sheetAfter = mobileSheet(wrapper)
      expect(sheetAfter.attributes('aria-expanded')).toBe('true')
    })

    it('drag-down gesture (>50px) collapses the sheet', async () => {
      const wrapper = mountSidebar()
      // First expand via tab click.
      const tabs = wrapper.findAll('[role="tab"]')
      const chatTab = tabs.find((t) =>
        t.text().includes('bottom_sheet_tab_chat'),
      )!
      await chatTab.trigger('click')
      let sheet = mobileSheet(wrapper)
      expect(sheet.attributes('aria-expanded')).toBe('true')
      // Now drag down.
      await sheet.trigger('touchstart', {
        touches: [{ clientY: 200 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchmove', {
        touches: [{ clientY: 270 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchend')
      sheet = mobileSheet(wrapper)
      expect(sheet.attributes('aria-expanded')).toBe('false')
    })

    it('small drag jitter (<50px) does not toggle the sheet', async () => {
      const wrapper = mountSidebar()
      const sheet = mobileSheet(wrapper)
      const beforeExpanded = sheet.attributes('aria-expanded')
      await sheet.trigger('touchstart', {
        touches: [{ clientY: 300 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchmove', {
        // only 20px up — under the 50px threshold
        touches: [{ clientY: 280 } as Touch],
      } as unknown as TouchEvent)
      await sheet.trigger('touchend')
      const sheetAfter = mobileSheet(wrapper)
      expect(sheetAfter.attributes('aria-expanded')).toBe(beforeExpanded)
    })
  })
})
