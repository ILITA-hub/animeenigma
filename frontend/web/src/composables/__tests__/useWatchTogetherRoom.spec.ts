/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.3.
 *
 * Vitest spec for `useWatchTogetherRoom` composable. Mocks the WebSocket
 * global + `@/api/watch-together`.getRoom + `@/stores/auth`.useAuthStore,
 * then drives the composable through every WS-lifecycle branch enumerated
 * in the plan's <behavior> section.
 *
 * 16 tests, mirroring Plan 02.3 §"Public-API spec coverage in tests" +
 * Task 2's four extra cases (heartbeat, emitChangePlayer, multi-subscriber,
 * disconnect-during-reconnect).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ──────────────────────────────────────────────────────────────────────────
//  Mocks (hoisted before SUT import).
// ──────────────────────────────────────────────────────────────────────────

vi.mock('@/api/watch-together', async () => {
  const actual = await vi.importActual<typeof import('@/api/watch-together')>(
    '@/api/watch-together',
  )
  return {
    ...actual,
    getRoom: vi.fn(),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: vi.fn(() => ({
    token: 'jwt.fake',
    user: { id: 'self-uuid' },
    isAuthenticated: true,
    // Guest-WT fields (logged-out invite-link join). Authenticated path is the
    // default for these tests; guest-specific behavior is covered separately.
    wtGuestToken: null,
    wtGuestUser: null,
    refreshAccessToken: vi.fn().mockResolvedValue(true),
    ensureGuestToken: vi.fn().mockResolvedValue('guest.jwt.fake'),
  })),
}))

// Imported AFTER vi.mock so the alias resolves to the stubs.
import { getRoom } from '@/api/watch-together'
import { useAuthStore } from '@/stores/auth'
import { useWatchTogetherRoom } from '../useWatchTogetherRoom'
import type {
  RoomSnapshot,
  Member,
  ChatMessage,
  PlaybackEventData,
  ChatMessageOutData,
  ChatReactionOutData,
  MemberJoinedData,
  MemberLeftData,
  RoomStateChangedData,
  PlaybackCorrectionData,
  ErrorData,
  Envelope,
} from '@/types/watch-together'

const getRoomSpy = getRoom as ReturnType<typeof vi.fn>

// ──────────────────────────────────────────────────────────────────────────
//  MockWebSocket — captures send() calls, lets tests drive open/close/message.
// ──────────────────────────────────────────────────────────────────────────

interface MockSocket {
  url: string
  readyState: number
  sendCalls: string[]
  onopen: ((e: Event) => void) | null
  onclose: ((e: CloseEvent) => void) | null
  onmessage: ((e: MessageEvent) => void) | null
  onerror: ((e: Event) => void) | null
  send(data: string): void
  close(): void
  simulateOpen(): void
  simulateMessage(envelope: unknown): void
  simulateClose(code?: number): void
}

let lastSocket: MockSocket | null = null
let socketsCreated: MockSocket[] = []

class MockWebSocket implements MockSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  url: string
  readyState = 0 // CONNECTING
  sendCalls: string[] = []
  onopen: ((e: Event) => void) | null = null
  onclose: ((e: CloseEvent) => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  onerror: ((e: Event) => void) | null = null

  constructor(url: string) {
    this.url = url
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    const self: MockWebSocket = this
    lastSocket = self
    socketsCreated.push(self)
  }

  send(data: string): void {
    this.sendCalls.push(data)
  }

  close(): void {
    if (this.readyState === MockWebSocket.CLOSED) return
    this.readyState = MockWebSocket.CLOSED
    if (this.onclose) {
      this.onclose({ code: 1000, reason: 'manual close' } as CloseEvent)
    }
  }

  simulateOpen(): void {
    this.readyState = MockWebSocket.OPEN
    if (this.onopen) this.onopen(new Event('open'))
  }

  simulateMessage(envelope: unknown): void {
    if (this.onmessage) {
      this.onmessage({ data: JSON.stringify(envelope) } as MessageEvent)
    }
  }

  simulateClose(code = 1006): void {
    this.readyState = MockWebSocket.CLOSED
    if (this.onclose) {
      this.onclose({ code, reason: '' } as CloseEvent)
    }
  }
}

// ──────────────────────────────────────────────────────────────────────────
//  Fixtures.
// ──────────────────────────────────────────────────────────────────────────

function makeSnapshot(overrides: Partial<RoomSnapshot> = {}): RoomSnapshot {
  return {
    room: {
      id: 'room-1',
      created_at: 1700000000,
      anime_id: 'a1',
      episode_id: 'e1',
      player: 'kodik',
      translation_id: 't1',
      playback_state: 'paused',
      playback_time: 0,
      playback_time_updated_at: 1700000000000,
      host_user_id: 'host-uuid',
    },
    members: [
      {
        user_id: 'self-uuid',
        meta: { username: 'self', avatar_url: '', joined_at: 1700000000, last_seen_at: 1700000000 },
      },
    ],
    messages: [],
    protocol_version: '1.0',
    ...overrides,
  }
}

// Wait for the microtask queue (composable's `connect()` awaits getRoom).
async function flushMicrotasks() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
  }
}

// ──────────────────────────────────────────────────────────────────────────
//  Test scaffolding.
// ──────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  vi.useFakeTimers()
  socketsCreated = []
  lastSocket = null
  getRoomSpy.mockReset()
  vi.stubGlobal('WebSocket', MockWebSocket)
  // Pin window.location for predictable WS URL construction.
  vi.stubGlobal('location', {
    protocol: 'https:',
    host: 'animeenigma.ru',
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests.
// ──────────────────────────────────────────────────────────────────────────

describe('useWatchTogetherRoom — connect + handshake', () => {
  it('Test 1: connect() calls getRoom then opens a WebSocket with token+room query params', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()

    expect(getRoomSpy).toHaveBeenCalledTimes(1)
    // Authenticated path passes undefined as the token arg (the apiClient
    // interceptor attaches the global token); guests pass their wtGuestToken.
    expect(getRoomSpy).toHaveBeenCalledWith('room-1', undefined)
    expect(lastSocket).not.toBeNull()
    expect(lastSocket!.url).toContain('/api/watch-together/ws')
    expect(lastSocket!.url).toContain('token=jwt.fake')
    expect(lastSocket!.url).toContain('room=room-1')
    expect(lastSocket!.url.startsWith('wss://')).toBe(true)
  })

  it('Test 1b: guest (logged-out) opens the WS with the guest token, not the global token', async () => {
    // Guest identity: the WT-only guest token lives in wtGuestToken; the
    // global `token` is null and isAuthenticated is false (no app-wide leak).
    const authMock = useAuthStore as unknown as ReturnType<typeof vi.fn>
    authMock.mockReturnValueOnce({
      token: null,
      user: null,
      isAuthenticated: false,
      wtGuestToken: 'guest.jwt.token',
      wtGuestUser: { id: 'guest-uuid', username: 'Guest-1234' },
      refreshAccessToken: vi.fn().mockResolvedValue(true),
      ensureGuestToken: vi.fn().mockResolvedValue('guest.jwt.token'),
    })
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()

    expect(lastSocket).not.toBeNull()
    expect(lastSocket!.url).toContain('token=guest.jwt.token')
    expect(lastSocket!.url).not.toContain('token=jwt.fake')
  })

  it('Test 2: first inbound room:snapshot populates room/members/messages', async () => {
    const snap = makeSnapshot({
      members: [
        {
          user_id: 'a',
          meta: { username: 'A', avatar_url: '', joined_at: 1, last_seen_at: 1 },
        },
        {
          user_id: 'b',
          meta: { username: 'B', avatar_url: '', joined_at: 1, last_seen_at: 1 },
        },
      ],
      messages: [{ id: 'm1', user_id: 'a', username: 'A', body: 'hi', ts: 1700000000000 }],
    })
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()

    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: snap } satisfies Envelope<RoomSnapshot>)

    expect(room.room.value?.id).toBe('room-1')
    expect(room.members.value.length).toBe(2)
    expect(room.messages.value.length).toBe(1)
    expect(room.connectionStatus.value).toBe('open')
  })
})

describe('useWatchTogetherRoom — emit methods', () => {
  async function setupOpenRoom() {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })
    return room
  }

  it('Test 3: emitPlay(42.5) sends {"type":"playback:play","data":{"time":42.5}}', async () => {
    const room = await setupOpenRoom()
    room.emitPlay(42.5)
    expect(lastSocket!.sendCalls).toContain(
      JSON.stringify({ type: 'playback:play', data: { time: 42.5 } }),
    )
  })

  it('Test 7: sendChat with empty / over-cap body is silently dropped', async () => {
    const room = await setupOpenRoom()
    const before = lastSocket!.sendCalls.length
    room.sendChat('')
    room.sendChat('   ')
    room.sendChat('a'.repeat(501))
    expect(lastSocket!.sendCalls.length).toBe(before)

    // Valid body: WAS sent.
    room.sendChat('hello')
    expect(lastSocket!.sendCalls.some((c) => c.includes('"body":"hello"'))).toBe(true)
  })

  it('Test 8: sendReaction with non-whitelist emoji is silently dropped', async () => {
    const room = await setupOpenRoom()
    const before = lastSocket!.sendCalls.length
    room.sendReaction('🚫')
    room.sendReaction('abc')
    expect(lastSocket!.sendCalls.length).toBe(before)

    // Whitelisted: WAS sent.
    room.sendReaction('🔥')
    expect(lastSocket!.sendCalls.some((c) => c.includes('"emoji":"🔥"'))).toBe(true)
  })

  it('Test 13: heartbeat() sends {"type":"presence:heartbeat","data":{}}', async () => {
    const room = await setupOpenRoom()
    room.heartbeat()
    expect(lastSocket!.sendCalls).toContain(
      JSON.stringify({ type: 'presence:heartbeat', data: {} }),
    )
  })

  it('Test 14: emitChangePlayer("animelib") sends correct envelope', async () => {
    const room = await setupOpenRoom()
    room.emitChangePlayer('animelib')
    expect(lastSocket!.sendCalls).toContain(
      JSON.stringify({ type: 'state:change_player', data: { player: 'animelib' } }),
    )
  })
})

describe('useWatchTogetherRoom — subscribe + echo guard', () => {
  async function setupOpenRoom() {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })
    return room
  }

  it('Test 4: playback:event with by_user_id !== self → handler fires; === self → handler skipped', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onPlaybackEvent(handler)

    const remote: PlaybackEventData = { kind: 'play', time: 10, by_user_id: 'other-uuid', server_ts: 0 }
    lastSocket!.simulateMessage({ type: 'playback:event', data: remote })
    expect(handler).toHaveBeenCalledTimes(1)

    const own: PlaybackEventData = { kind: 'play', time: 20, by_user_id: 'self-uuid', server_ts: 0 }
    lastSocket!.simulateMessage({ type: 'playback:event', data: own })
    expect(handler).toHaveBeenCalledTimes(1) // NOT called again
  })

  it('Test 15: multiple onChatMessage subscribers fire; unsubscribe detaches only its handler', async () => {
    const room = await setupOpenRoom()
    const h1 = vi.fn()
    const h2 = vi.fn()
    const off1 = room.onChatMessage(h1)
    room.onChatMessage(h2)

    const msg: ChatMessage = { id: 'x', user_id: 'other', username: 'O', body: 'hi', ts: 0 }
    const out: ChatMessageOutData = { message: msg }
    lastSocket!.simulateMessage({ type: 'chat:message', data: out })

    expect(h1).toHaveBeenCalledTimes(1)
    expect(h2).toHaveBeenCalledTimes(1)

    off1()
    lastSocket!.simulateMessage({ type: 'chat:message', data: out })
    expect(h1).toHaveBeenCalledTimes(1) // unchanged
    expect(h2).toHaveBeenCalledTimes(2)
  })

  it('chat:message inbound: appends to messages and fires onChatMessage', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onChatMessage(handler)

    const msg: ChatMessage = { id: 'x', user_id: 'other', username: 'O', body: 'hi', ts: 0 }
    lastSocket!.simulateMessage({ type: 'chat:message', data: { message: msg } satisfies ChatMessageOutData })

    expect(room.messages.value.length).toBe(1)
    expect(handler).toHaveBeenCalledWith(msg)
  })

  it('member:joined inbound: appends to members and fires onMemberJoined', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onMemberJoined(handler)

    const newMember: Member = {
      user_id: 'new-user',
      meta: { username: 'N', avatar_url: '', joined_at: 0, last_seen_at: 0 },
    }
    const payload: MemberJoinedData = { user_id: 'new-user', member: newMember.meta }
    lastSocket!.simulateMessage({ type: 'member:joined', data: payload })

    expect(room.members.value.some((m) => m.user_id === 'new-user')).toBe(true)
    expect(handler).toHaveBeenCalledWith(payload)
  })

  it('member:left inbound: removes from members and fires onMemberLeft', async () => {
    const initialSnap = makeSnapshot({
      members: [
        { user_id: 'a', meta: { username: 'A', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
        { user_id: 'b', meta: { username: 'B', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
      ],
    })
    getRoomSpy.mockReset()
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: initialSnap })

    const handler = vi.fn()
    room.onMemberLeft(handler)
    const payload: MemberLeftData = { user_id: 'a' }
    lastSocket!.simulateMessage({ type: 'member:left', data: payload })

    expect(room.members.value.some((m) => m.user_id === 'a')).toBe(false)
    expect(room.members.value.length).toBe(1)
    expect(handler).toHaveBeenCalledWith(payload)
  })

  it('room:state_changed echo guard: own event suppressed, remote event fires', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onStateChanged(handler)

    const own: RoomStateChangedData = { field: 'player', value: 'animelib', by_user_id: 'self-uuid' }
    lastSocket!.simulateMessage({ type: 'room:state_changed', data: own })
    expect(handler).not.toHaveBeenCalled()

    const remote: RoomStateChangedData = { field: 'player', value: 'animelib', by_user_id: 'other' }
    lastSocket!.simulateMessage({ type: 'room:state_changed', data: remote })
    expect(handler).toHaveBeenCalledTimes(1)
  })

  it('chat:reaction inbound: appended to reactions ring buffer + onReaction fires', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onReaction(handler)
    const payload: ChatReactionOutData = { user_id: 'other', emoji: '🔥' }
    lastSocket!.simulateMessage({ type: 'chat:reaction', data: payload })
    expect(room.reactions.value.length).toBe(1)
    expect(room.reactions.value[0].emoji).toBe('🔥')
    expect(handler).toHaveBeenCalledWith(payload)
  })

  it('playback:correction inbound: fires onCorrection', async () => {
    const room = await setupOpenRoom()
    const handler = vi.fn()
    room.onCorrection(handler)
    const payload: PlaybackCorrectionData = { time: 42, server_ts: 1700000000000 }
    lastSocket!.simulateMessage({ type: 'playback:correction', data: payload })
    expect(handler).toHaveBeenCalledWith(payload)
  })
})

describe('useWatchTogetherRoom — reconnect backoff', () => {
  it('Test 5: reconnect backoff is 1s, 2s, 4s, 8s, 16s, 30s; further closes stay 30s', async () => {
    getRoomSpy.mockResolvedValue(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    // FIRST socket established.
    expect(socketsCreated.length).toBe(1)

    // Close immediately (before any snapshot received → backoff index 0 = 1s).
    lastSocket!.simulateClose()
    // First reconnect: 1000ms.
    await vi.advanceTimersByTimeAsync(999)
    expect(socketsCreated.length).toBe(1)
    await vi.advanceTimersByTimeAsync(2)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(2)

    // Second reconnect: 2000ms.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(1999)
    expect(socketsCreated.length).toBe(2)
    await vi.advanceTimersByTimeAsync(2)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(3)

    // Third: 4000ms.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(4001)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(4)

    // Fourth: 8000ms.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(8001)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(5)

    // Fifth: 16000ms.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(16001)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(6)

    // Sixth: capped at 30000ms.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(29999)
    expect(socketsCreated.length).toBe(6)
    await vi.advanceTimersByTimeAsync(2)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(7)

    // Seventh: still 30000ms (cap).
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(30001)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(8)
  })

  it('Test 6: room:snapshot after reconnect resets backoff index to 0', async () => {
    getRoomSpy.mockResolvedValue(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()

    // Burn through to backoff index 2 (4s).
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(1001)
    await flushMicrotasks()
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(2001)
    await flushMicrotasks()
    // Now we have 3 sockets created.
    expect(socketsCreated.length).toBe(3)

    // Third socket: successful snapshot → resets backoff to 0.
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    // Close again → should reconnect after 1000ms (not 4000ms).
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(1001)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(4)
  })

  it('Test 16: disconnect() during pending reconnect cancels the pending reconnect', async () => {
    getRoomSpy.mockResolvedValue(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(1)

    lastSocket!.simulateClose()
    // Half-way through the 1s backoff window: disconnect.
    await vi.advanceTimersByTimeAsync(500)
    room.disconnect()
    await vi.advanceTimersByTimeAsync(5000)
    await flushMicrotasks()
    // No new socket should have been opened after disconnect.
    expect(socketsCreated.length).toBe(1)
    expect(room.connectionStatus.value).toBe('closed')
  })
})

describe('useWatchTogetherRoom — disconnect + error semantics', () => {
  it('Test 9: disconnect() closes the socket', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    room.disconnect()
    expect(lastSocket!.readyState).toBe(MockWebSocket.CLOSED)
    expect(room.connectionStatus.value).toBe('closed')
  })

  it('Test 10: AUTH_EXPIRED error sets connectionStatus=failed and prevents reconnect', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    const errHandler = vi.fn()
    room.onError(errHandler)
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    const errPayload: ErrorData = { code: 'AUTH_EXPIRED', message: 'jwt expired' }
    lastSocket!.simulateMessage({ type: 'error', data: errPayload })

    expect(errHandler).toHaveBeenCalledWith(errPayload)
    expect(room.connectionStatus.value).toBe('failed')

    // Close the socket — composable must NOT reconnect.
    lastSocket!.simulateClose()
    await vi.advanceTimersByTimeAsync(60000)
    await flushMicrotasks()
    expect(socketsCreated.length).toBe(1)
  })

  // ── Phase 05 (polish) Plan 05.5 — onAuthExpired subscriber sugar ──
  //
  // Three tests lock the contract that the dedicated `onAuthExpired` channel
  // wraps onError + filters for ERR_AUTH_EXPIRED. The view code consumes
  // this in preference to a catch-all onError branch so the auth-expired
  // modal stays decoupled from the state-error toast trio.

  it('Test 20: onAuthExpired fires when an AUTH_EXPIRED frame arrives', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    const authHandler = vi.fn()
    room.onAuthExpired(authHandler)
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    const errPayload: ErrorData = { code: 'AUTH_EXPIRED', message: 'jwt expired' }
    lastSocket!.simulateMessage({ type: 'error', data: errPayload })

    expect(authHandler).toHaveBeenCalledTimes(1)
  })

  it('Test 21: onAuthExpired does NOT fire for other error codes', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    const authHandler = vi.fn()
    room.onAuthExpired(authHandler)
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    lastSocket!.simulateMessage({
      type: 'error',
      data: { code: 'CAPACITY_FULL', message: 'room full' } satisfies ErrorData,
    })
    lastSocket!.simulateMessage({
      type: 'error',
      data: { code: 'EPISODE_UNAVAILABLE', message: 'no ep' } satisfies ErrorData,
    })
    expect(authHandler).not.toHaveBeenCalled()
  })

  it('Test 22: onAuthExpired returns an unsubscriber that prevents further calls', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    const authHandler = vi.fn()
    const off = room.onAuthExpired(authHandler)
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    off()
    lastSocket!.simulateMessage({
      type: 'error',
      data: { code: 'AUTH_EXPIRED', message: 'jwt expired' } satisfies ErrorData,
    })
    expect(authHandler).not.toHaveBeenCalled()
  })
})

describe('useWatchTogetherRoom — reaction prune + snapshot replay', () => {
  it('Test 11: reactions ring buffer entries are pruned after 5000ms', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()
    lastSocket!.simulateMessage({ type: 'room:snapshot', data: makeSnapshot() })

    lastSocket!.simulateMessage({
      type: 'chat:reaction',
      data: { user_id: 'other', emoji: '🔥' } satisfies ChatReactionOutData,
    })
    expect(room.reactions.value.length).toBe(1)

    // Less than 5s: still present.
    await vi.advanceTimersByTimeAsync(4500)
    expect(room.reactions.value.length).toBe(1)

    // Pass the 5s threshold + one prune tick.
    await vi.advanceTimersByTimeAsync(2000)
    expect(room.reactions.value.length).toBe(0)
  })

  it('Test 12: snapshot replay OVERWRITES previous members (does not merge)', async () => {
    getRoomSpy.mockResolvedValueOnce(makeSnapshot())
    const room = useWatchTogetherRoom('room-1')
    await room.connect()
    await flushMicrotasks()
    lastSocket!.simulateOpen()

    // First snapshot: A + B.
    lastSocket!.simulateMessage({
      type: 'room:snapshot',
      data: makeSnapshot({
        members: [
          { user_id: 'A', meta: { username: 'A', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
          { user_id: 'B', meta: { username: 'B', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
        ],
      }),
    })
    expect(room.members.value.length).toBe(2)

    // Second snapshot: only C — must NOT merge into [A,B,C].
    lastSocket!.simulateMessage({
      type: 'room:snapshot',
      data: makeSnapshot({
        members: [
          { user_id: 'C', meta: { username: 'C', avatar_url: '', joined_at: 0, last_seen_at: 0 } },
        ],
      }),
    })
    expect(room.members.value.length).toBe(1)
    expect(room.members.value[0].user_id).toBe('C')
  })
})
