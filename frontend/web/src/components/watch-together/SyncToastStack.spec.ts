/**
 * Workstream watch-together — Phase 03 (player-sync) Plan 03.5 Task 2.
 *
 * Vitest spec for SyncToastStack.vue. Verifies:
 *   1. Mounts with no toasts (empty stack on first render).
 *   2. Subscribes to room.onPlaybackEvent on mount; unsubscribes on unmount.
 *   3. Inbound `play` event → renders the `sync_toast_played` label.
 *   4. Inbound `pause` event → renders the `sync_toast_paused` label.
 *   5. Inbound `seek` event → renders the `sync_toast_seeked` label with
 *      mm:ss-formatted time.
 *   6. Username falls back to `someone` when by_user_id isn't in members.
 *   7. Toasts auto-remove after TOAST_LIFETIME_MS (fake timers).
 *   8. Stack caps at 3 entries — adding a 4th drops the oldest.
 *   9. Time formatter renders mm:ss (zero-padded, no hours).
 *  10. Wrapper has `pointer-events-none` (click-through to player).
 *
 * `vue-i18n` is stubbed so t() returns `key::{params}` so we can assert on
 * interpolation without spinning up a real i18n instance.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref, nextTick } from 'vue'

// ── Mocks ────────────────────────────────────────────────────────────────

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import SyncToastStack from './SyncToastStack.vue'
import type {
  WatchTogetherRoomHandle,
  ConnectionStatus,
  ReactionEvent,
} from '@/composables/useWatchTogetherRoom'
import type {
  Room,
  Member,
  ChatMessage,
  PlaybackEventData,
} from '@/api/watch-together'

// Build a fake `WatchTogetherRoomHandle` that captures the playback handler
// so tests can drive events directly. `members.value` is a writable ref so
// individual tests can mutate the roster mid-test.
function makeFakeRoom(members: Member[] = []) {
  let captured: ((e: PlaybackEventData) => void) | null = null
  const unsub = vi.fn()
  const onPlaybackEvent = vi.fn(
    (handler: (e: PlaybackEventData) => void) => {
      captured = handler
      return unsub
    },
  )
  const fakeRoom: Partial<WatchTogetherRoomHandle> = {
    room: ref<Room | null>(null),
    members: ref<Member[]>(members),
    messages: ref<ChatMessage[]>([]),
    reactions: ref<ReactionEvent[]>([]),
    connectionStatus: ref<ConnectionStatus>('open'),
    lastError: ref(null),
    onPlaybackEvent,
    // The component only uses onPlaybackEvent + members; the rest are
    // never invoked. We cast through unknown to satisfy the type without
    // building out every no-op method.
  }
  return {
    handle: fakeRoom as unknown as WatchTogetherRoomHandle,
    fire: (e: PlaybackEventData) => captured?.(e),
    unsub,
    onPlaybackEvent,
  }
}

function makeMember(id: string, username: string): Member {
  return {
    user_id: id,
    meta: {
      username,
      avatar_url: '',
      joined_at: 1_700_000_000,
      last_seen_at: 1_700_000_000,
    },
  }
}

function makeEvent(
  kind: 'play' | 'pause' | 'seek',
  by_user_id: string,
  time: number,
): PlaybackEventData {
  return { kind, time, by_user_id, server_ts: Date.now() }
}

describe('SyncToastStack', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('mounts with no toasts', () => {
    const { handle } = makeFakeRoom()
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    // The transition-group is rendered but contains no toast children.
    expect(wrapper.text().trim()).toBe('')
  })

  it('subscribes to room.onPlaybackEvent on mount and unsubscribes on unmount', () => {
    const { handle, onPlaybackEvent, unsub } = makeFakeRoom()
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    expect(onPlaybackEvent).toHaveBeenCalledTimes(1)
    expect(unsub).not.toHaveBeenCalled()
    wrapper.unmount()
    expect(unsub).toHaveBeenCalledTimes(1)
  })

  it('renders sync_toast_played label for inbound play event', async () => {
    const { handle, fire } = makeFakeRoom([makeMember('u1', 'Alice')])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('play', 'u1', 0))
    await nextTick()
    expect(wrapper.text()).toContain('watch_together.sync_toast_played')
    expect(wrapper.text()).toContain('Alice')
  })

  it('renders sync_toast_paused label for inbound pause event', async () => {
    const { handle, fire } = makeFakeRoom([makeMember('u2', 'Bob')])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('pause', 'u2', 12.4))
    await nextTick()
    expect(wrapper.text()).toContain('watch_together.sync_toast_paused')
    expect(wrapper.text()).toContain('Bob')
  })

  it('renders sync_toast_seeked label with mm:ss-formatted time for inbound seek', async () => {
    const { handle, fire } = makeFakeRoom([makeMember('u3', 'Carol')])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    // 754 seconds = 12:34
    fire(makeEvent('seek', 'u3', 754))
    await nextTick()
    const text = wrapper.text()
    expect(text).toContain('watch_together.sync_toast_seeked')
    expect(text).toContain('Carol')
    expect(text).toContain('12:34')
  })

  it('falls back to `someone` when by_user_id is not in members', async () => {
    const { handle, fire } = makeFakeRoom([])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('play', 'unknown-user', 0))
    await nextTick()
    expect(wrapper.text()).toContain('someone')
  })

  it('removes a toast after TOAST_LIFETIME_MS', async () => {
    const { handle, fire } = makeFakeRoom([makeMember('u1', 'Alice')])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('play', 'u1', 0))
    await nextTick()
    expect(wrapper.text()).toContain('Alice')

    // Advance past the 2000ms lifetime.
    vi.advanceTimersByTime(2001)
    await nextTick()
    expect(wrapper.text()).not.toContain('Alice')
  })

  it('caps the stack at 3 — a 4th event drops the oldest', async () => {
    const { handle, fire } = makeFakeRoom([
      makeMember('u1', 'Alice'),
      makeMember('u2', 'Bob'),
      makeMember('u3', 'Carol'),
      makeMember('u4', 'Dan'),
    ])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('play', 'u1', 0))
    fire(makeEvent('pause', 'u2', 1))
    fire(makeEvent('seek', 'u3', 60))
    fire(makeEvent('play', 'u4', 90))
    await nextTick()

    const text = wrapper.text()
    // Most recent 3 (Bob, Carol, Dan) should be visible; oldest (Alice) gone.
    expect(text).not.toContain('Alice')
    expect(text).toContain('Bob')
    expect(text).toContain('Carol')
    expect(text).toContain('Dan')
  })

  it('formats 300 seconds as 05:00 and 754 seconds as 12:34', async () => {
    const { handle, fire } = makeFakeRoom([
      makeMember('u1', 'Alice'),
      makeMember('u2', 'Bob'),
    ])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('seek', 'u1', 300))
    fire(makeEvent('seek', 'u2', 754))
    await nextTick()
    const text = wrapper.text()
    expect(text).toContain('05:00')
    expect(text).toContain('12:34')
  })

  it('outer wrapper has pointer-events-none', () => {
    const { handle } = makeFakeRoom()
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    expect(wrapper.classes()).toContain('pointer-events-none')
  })

  it('uses only font-medium / font-semibold weights (no font-bold)', async () => {
    const { handle, fire } = makeFakeRoom([makeMember('u1', 'Alice')])
    const wrapper = mount(SyncToastStack, { props: { room: handle } })
    fire(makeEvent('play', 'u1', 0))
    await nextTick()
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
    expect(html).toMatch(/font-medium|font-semibold/)
  })
})
