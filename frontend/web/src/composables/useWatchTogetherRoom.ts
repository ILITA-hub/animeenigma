/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.3.
 *
 * `useWatchTogetherRoom(roomId)` — single source of truth for a room's
 * reactive state, WebSocket lifecycle, and typed emit/subscribe surface.
 *
 *   - Open WS on `connect()` (REST `getRoom` first, then upgrade).
 *   - Close on `disconnect()` (also auto-fires on `onUnmounted` if invoked
 *     inside a Vue setup context).
 *   - Auto-reconnect with exponential backoff `[1s, 2s, 4s, 8s, 16s, 30s]`,
 *     capped at 30s; index resets to 0 on every successful `room:snapshot`.
 *   - Snapshot replay: every `room:snapshot` REPLACES `room` / `members` /
 *     `messages` (not merges) — the canonical reconnect contract.
 *   - Re-emission guard: when an inbound event's `by_user_id` matches
 *     `useAuthStore().user.id`, the matching `on*` handlers are NOT fired.
 *     (Applies to `playback:event` + `room:state_changed`; chat/reaction
 *     deliberately fire for everyone — the server is the persistence
 *     anchor, and the sender's UI shows their own message as soon as the
 *     echo arrives.)
 *   - Reaction ring buffer: each `chat:reaction` enters `reactions.value`
 *     for 5s, then is pruned by a 1Hz interval so the BurstOverlay can
 *     iterate without managing its own state.
 *
 * Downstream consumers (RoomSidebar, ChatPanel, ReactionPalette,
 * ReactionBurstOverlay, WatchTogetherView) all destructure this composable
 * and treat its returned refs as read-only; emit methods perform validation
 * (chat length, reaction whitelist) so callers don't repeat it.
 */

import { ref, type Ref, getCurrentInstance, onUnmounted } from 'vue'

import { useAuthStore } from '@/stores/auth'
import {
  getRoom,
  MSG_PLAYBACK_PLAY,
  MSG_PLAYBACK_PAUSE,
  MSG_PLAYBACK_SEEK,
  MSG_PLAYBACK_TIME_TICK,
  MSG_STATE_CHANGE_EPISODE,
  MSG_STATE_CHANGE_PLAYER,
  MSG_STATE_CHANGE_TRANSLATION,
  MSG_CHAT_MESSAGE,
  MSG_CHAT_REACTION,
  MSG_PRESENCE_HEARTBEAT,
  MSG_ROOM_SNAPSHOT,
  MSG_ROOM_STATE_CHANGED,
  MSG_PLAYBACK_EVENT,
  MSG_PLAYBACK_CORRECTION,
  MSG_MEMBER_JOINED,
  MSG_MEMBER_LEFT,
  MSG_CHAT_MESSAGE_OUT,
  MSG_CHAT_REACTION_OUT,
  MSG_ROOM_CLOSED,
  MSG_ERROR,
  ERR_AUTH_EXPIRED,
  ERR_CAPACITY_FULL,
  ERR_ROOM_NOT_FOUND,
  REACTION_WHITELIST,
  type Room,
  type Member,
  type ChatMessage,
  type RoomSnapshot,
  type PlaybackEventData,
  type PlaybackCorrectionData,
  type RoomStateChangedData,
  type MemberJoinedData,
  type MemberLeftData,
  type ChatMessageOutData,
  type ChatReactionOutData,
  type RoomClosedData,
  type ErrorData,
  type PlayerKind,
  type ErrorCode,
} from '@/api/watch-together'

/* ──────────────────────────────────────────────────────────────────────── */
/*  Public types.                                                           */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Connection lifecycle states. UI binds `connectionStatus` to surface a
 * "Reconnecting…" indicator (when 'reconnecting') and an "Unable to
 * reconnect — refresh" empty-state (when 'failed').
 */
export type ConnectionStatus =
  | 'idle'
  | 'connecting'
  | 'open'
  | 'reconnecting'
  | 'closed'
  | 'failed'

/**
 * Local-only reaction event for the BurstOverlay. `id` is a monotonically
 * increasing counter so Vue's keyed v-for doesn't reuse DOM across distinct
 * bursts; `ts` is wall-clock ms (used by the 5s prune loop).
 */
export interface ReactionEvent {
  id: number
  emoji: string
  user_id: string
  ts: number
}

/** Public surface of `useWatchTogetherRoom`. Locked — Phase 3 builds against it. */
export interface UseWatchTogetherRoomReturn {
  // Reactive state.
  room: Ref<Room | null>
  members: Ref<Member[]>
  messages: Ref<ChatMessage[]>
  reactions: Ref<ReactionEvent[]>
  connectionStatus: Ref<ConnectionStatus>
  lastError: Ref<{ code: ErrorCode | string; message?: string; hint?: string } | null>

  // Emit (client → server). Validation lives here so callers don't repeat it.
  emitPlay(time: number): void
  emitPause(time: number): void
  emitSeek(time: number): void
  emitTimeTick(time: number): void
  emitChangeEpisode(episode_id: string): void
  emitChangePlayer(player: PlayerKind): void
  emitChangeTranslation(translation_id: string): void
  sendChat(body: string): void
  sendReaction(emoji: string): void
  heartbeat(): void

  // Subscribe (server → client). Each returns its own unsubscribe.
  onPlaybackEvent(handler: (e: PlaybackEventData) => void): () => void
  onStateChanged(handler: (e: RoomStateChangedData) => void): () => void
  onChatMessage(handler: (m: ChatMessage) => void): () => void
  onReaction(handler: (r: ChatReactionOutData) => void): () => void
  onMemberJoined(handler: (m: MemberJoinedData) => void): () => void
  onMemberLeft(handler: (m: MemberLeftData) => void): () => void
  onCorrection(handler: (c: PlaybackCorrectionData) => void): () => void
  onError(handler: (e: ErrorData) => void): () => void
  onRoomClosed(handler: (r: RoomClosedData) => void): () => void

  // Control.
  connect(): Promise<void>
  disconnect(): void
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Internals.                                                              */
/* ──────────────────────────────────────────────────────────────────────── */

// Backoff schedule per WT-SHELL-02 / design doc §"Auto-reconnect UX".
const BACKOFF_MS = [1000, 2000, 4000, 8000, 16000, 30000] as const

// Reaction ring buffer prune window. Matches the 3-5s designer intent from
// the watch-together design doc §Chat/reactions; we use 5s for the buffer
// (BurstOverlay animations are 3s, leaving 2s of slack for keyed v-for
// transitions to finish before the entry disappears).
const REACTION_TTL_MS = 5000
const REACTION_PRUNE_INTERVAL_MS = 1000

// Maximum chat body length (WT-FOUND-05). Mirrors the server-side cap.
const CHAT_MAX_LEN = 500

// Cast for runtime indexing of REACTION_WHITELIST. The constant is typed as
// readonly tuple of literals; `.includes(emoji)` against a `string` parameter
// would force callers to narrow first. The cast keeps the type strict at the
// callsite while letting us validate arbitrary user input.
const REACTION_SET = new Set<string>(REACTION_WHITELIST as readonly string[])

type Handler<T> = (payload: T) => void

interface HandlerRegistry {
  playbackEvent: Set<Handler<PlaybackEventData>>
  stateChanged: Set<Handler<RoomStateChangedData>>
  chatMessage: Set<Handler<ChatMessage>>
  reaction: Set<Handler<ChatReactionOutData>>
  memberJoined: Set<Handler<MemberJoinedData>>
  memberLeft: Set<Handler<MemberLeftData>>
  correction: Set<Handler<PlaybackCorrectionData>>
  error: Set<Handler<ErrorData>>
  roomClosed: Set<Handler<RoomClosedData>>
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Composable.                                                             */
/* ──────────────────────────────────────────────────────────────────────── */

export function useWatchTogetherRoom(roomId: string): UseWatchTogetherRoomReturn {
  const auth = useAuthStore()
  // Captured once: the auth ID is stable for the room's lifetime. If it
  // expires mid-session, the server emits AUTH_EXPIRED and the composable
  // transitions to 'failed' (no reconnect, view redirects to /login).
  const ownUserId: string | null = auth.user?.id ?? null

  // ── Reactive state ──
  const room = ref<Room | null>(null)
  const members = ref<Member[]>([])
  const messages = ref<ChatMessage[]>([])
  const reactions = ref<ReactionEvent[]>([])
  const connectionStatus = ref<ConnectionStatus>('idle')
  const lastError = ref<{ code: ErrorCode | string; message?: string; hint?: string } | null>(null)

  // ── Subscriber registry ──
  const handlers: HandlerRegistry = {
    playbackEvent: new Set(),
    stateChanged: new Set(),
    chatMessage: new Set(),
    reaction: new Set(),
    memberJoined: new Set(),
    memberLeft: new Set(),
    correction: new Set(),
    error: new Set(),
    roomClosed: new Set(),
  }

  // ── Lifecycle state (mutable across reconnects) ──
  let socket: WebSocket | null = null
  let backoffIndex = 0
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let pruneTimer: ReturnType<typeof setInterval> | null = null
  let stopped = false // set by disconnect() or by terminal error (AUTH_EXPIRED, etc.)
  let nextReactionId = 0
  // Dedupe noisy "bad frame" warnings — once per type per session.
  const warnedTypes = new Set<string>()

  /* ──────── Helpers ──────── */

  function send(envelope: { type: string; data: unknown }): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      // Silently drop. Phase 3 view UX never produces inbound traffic until
      // connectionStatus==='open'; a warn would be noise during reconnect.
      return
    }
    socket.send(JSON.stringify(envelope))
  }

  function buildWsUrl(): string {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const token = encodeURIComponent(auth.token ?? '')
    const id = encodeURIComponent(roomId)
    return `${proto}//${location.host}/api/watch-together/ws?token=${token}&room=${id}`
  }

  function clearReconnectTimer() {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function scheduleReconnect() {
    if (stopped) return
    const delay = BACKOFF_MS[Math.min(backoffIndex, BACKOFF_MS.length - 1)]
    connectionStatus.value = 'reconnecting'
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      if (stopped) return
      // Advance the index AFTER scheduling so the next close uses the next
      // step. Snapshot receipt will reset it.
      backoffIndex = Math.min(backoffIndex + 1, BACKOFF_MS.length - 1)
      openSocket()
    }, delay)
  }

  function startPruneLoop() {
    if (pruneTimer !== null) return
    pruneTimer = setInterval(() => {
      const cutoff = Date.now() - REACTION_TTL_MS
      if (reactions.value.length === 0) return
      reactions.value = reactions.value.filter((r) => r.ts > cutoff)
    }, REACTION_PRUNE_INTERVAL_MS)
  }

  function stopPruneLoop() {
    if (pruneTimer !== null) {
      clearInterval(pruneTimer)
      pruneTimer = null
    }
  }

  /* ──────── Dispatch ──────── */

  function applySnapshot(snap: RoomSnapshot) {
    // Strict overwrite — see plan §"Snapshot replay" for the contract.
    room.value = snap.room
    members.value = [...snap.members]
    messages.value = [...snap.messages]
    // Reset backoff on every successful snapshot (whether first or post-reconnect).
    backoffIndex = 0
  }

  function dispatch(raw: string) {
    let env: { type?: unknown; data?: unknown }
    try {
      env = JSON.parse(raw) as { type?: unknown; data?: unknown }
    } catch {
      if (!warnedTypes.has('__parse_error__')) {
        warnedTypes.add('__parse_error__')
        // eslint-disable-next-line no-console
        console.warn('[watch-together] failed to parse frame', raw.slice(0, 80))
      }
      return
    }
    if (!env || typeof env.type !== 'string') {
      return
    }
    const type = env.type
    const data = env.data

    switch (type) {
      case MSG_ROOM_SNAPSHOT: {
        applySnapshot(data as RoomSnapshot)
        return
      }
      case MSG_ROOM_STATE_CHANGED: {
        const payload = data as RoomStateChangedData
        // Mutate room field IF we recognize it (string-keyed, value typed as
        // unknown on the wire). Recognition list mirrors the three state:*
        // change handlers in the backend (services/watch-together/internal/
        // service/state.go).
        if (room.value) {
          if (payload.field === 'episode_id' && typeof payload.value === 'string') {
            room.value.episode_id = payload.value
          } else if (payload.field === 'player' && typeof payload.value === 'string') {
            room.value.player = payload.value as PlayerKind
          } else if (payload.field === 'translation_id' && typeof payload.value === 'string') {
            room.value.translation_id = payload.value
          }
        }
        if (ownUserId && payload.by_user_id === ownUserId) {
          // Echo of our own change — state already updated locally before emit.
          return
        }
        handlers.stateChanged.forEach((h) => h(payload))
        return
      }
      case MSG_PLAYBACK_EVENT: {
        const payload = data as PlaybackEventData
        // Update canonical room playback fields for late readers (e.g.
        // overlays computing remaining-time without needing their own state).
        if (room.value) {
          if (payload.kind === 'play') {
            room.value.playback_state = 'playing'
          } else if (payload.kind === 'pause') {
            room.value.playback_state = 'paused'
          }
          room.value.playback_time = payload.time
          room.value.playback_time_updated_at = payload.server_ts
        }
        if (ownUserId && payload.by_user_id === ownUserId) {
          return
        }
        handlers.playbackEvent.forEach((h) => h(payload))
        return
      }
      case MSG_PLAYBACK_CORRECTION: {
        // Personalized message: no echo guard (server never sends this to the
        // user whose drift was being corrected on a NON-drift basis — by
        // definition the recipient is the drift source).
        const payload = data as PlaybackCorrectionData
        handlers.correction.forEach((h) => h(payload))
        return
      }
      case MSG_MEMBER_JOINED: {
        const payload = data as MemberJoinedData
        // Idempotent — backend may resend on reconnect race.
        if (!members.value.some((m) => m.user_id === payload.user_id)) {
          members.value = [...members.value, { user_id: payload.user_id, meta: payload.member }]
        }
        handlers.memberJoined.forEach((h) => h(payload))
        return
      }
      case MSG_MEMBER_LEFT: {
        const payload = data as MemberLeftData
        members.value = members.value.filter((m) => m.user_id !== payload.user_id)
        handlers.memberLeft.forEach((h) => h(payload))
        return
      }
      case MSG_CHAT_MESSAGE_OUT: {
        // Wire string is identical to MSG_CHAT_MESSAGE — payload shape is the
        // discriminator. Inbound chat shape is {message: ChatMessage}.
        const payload = data as ChatMessageOutData
        if (payload?.message) {
          messages.value = [...messages.value, payload.message]
          handlers.chatMessage.forEach((h) => h(payload.message))
        }
        return
      }
      case MSG_CHAT_REACTION_OUT: {
        const payload = data as ChatReactionOutData
        const evt: ReactionEvent = {
          id: ++nextReactionId,
          emoji: payload.emoji,
          user_id: payload.user_id,
          ts: Date.now(),
        }
        reactions.value = [...reactions.value, evt]
        handlers.reaction.forEach((h) => h(payload))
        return
      }
      case MSG_ROOM_CLOSED: {
        const payload = (data ?? { reason: 'unknown' }) as RoomClosedData
        // Terminal — server will close right after this frame.
        stopped = true
        clearReconnectTimer()
        connectionStatus.value = 'closed'
        handlers.roomClosed.forEach((h) => h(payload))
        return
      }
      case MSG_ERROR: {
        const payload = data as ErrorData
        lastError.value = payload
        // Terminal error codes — do NOT reconnect.
        if (
          payload.code === ERR_AUTH_EXPIRED ||
          payload.code === ERR_CAPACITY_FULL ||
          payload.code === ERR_ROOM_NOT_FOUND
        ) {
          stopped = true
          clearReconnectTimer()
          connectionStatus.value = 'failed'
        }
        handlers.error.forEach((h) => h(payload))
        return
      }
      default: {
        // Unknown type — log once per type, drop. The protocol may add
        // forward-compatible types; we don't crash on them.
        if (!warnedTypes.has(type)) {
          warnedTypes.add(type)
          // eslint-disable-next-line no-console
          console.warn('[watch-together] unknown frame type', type)
        }
        return
      }
    }
  }

  /* ──────── Socket lifecycle ──────── */

  function openSocket() {
    if (stopped) return
    connectionStatus.value = 'connecting'
    const ws = new WebSocket(buildWsUrl())
    socket = ws

    ws.onopen = () => {
      // We don't transition to 'open' yet — that happens on first
      // `room:snapshot`. But the WebSocket itself is hot; emit methods
      // would technically succeed. To keep the UI status truthful, flip
      // to 'open' here — snapshot replay still resets backoff.
      connectionStatus.value = 'open'
    }

    ws.onmessage = (e) => {
      const data = typeof e.data === 'string' ? e.data : String(e.data)
      dispatch(data)
    }

    ws.onclose = () => {
      if (stopped) {
        connectionStatus.value = 'closed'
        return
      }
      // Unexpected close → reconnect with current backoff index.
      scheduleReconnect()
    }

    ws.onerror = () => {
      // We rely on the subsequent `onclose` to drive reconnect. Some
      // browsers fire `onerror` before `onclose`, others only `onclose`.
      // No need to act here — recording would be redundant.
    }
  }

  async function connect(): Promise<void> {
    if (stopped) {
      // Re-entrant connect after a failure: reset the kill switch and let
      // the caller try again. (The view's "Refresh" button might trigger
      // this — keeping the option open is cheap.)
      stopped = false
    }
    connectionStatus.value = 'connecting'
    try {
      // REST pre-fetch — handles 410 Gone before we burn a WS upgrade.
      // The snapshot from REST is ALSO mirrored into our refs so the UI
      // can render immediately; the WS will deliver an authoritative
      // `room:snapshot` shortly after, which simply replaces these values.
      const snap = await getRoom(roomId)
      applySnapshot(snap)
    } catch (err) {
      connectionStatus.value = 'failed'
      // Surface as a synthetic error envelope so subscribers (and
      // lastError) see a uniform shape. Errors NOT in our ErrorCode
      // union flow through with code = 'REST_FAILED'.
      const errPayload: ErrorData = {
        code: 'REST_FAILED',
        message: err instanceof Error ? err.message : 'failed to fetch room',
      }
      lastError.value = errPayload
      handlers.error.forEach((h) => h(errPayload))
      return
    }
    startPruneLoop()
    openSocket()
  }

  function disconnect() {
    stopped = true
    clearReconnectTimer()
    stopPruneLoop()
    if (socket) {
      try {
        socket.close()
      } catch {
        // best-effort
      }
      socket = null
    }
    connectionStatus.value = 'closed'
  }

  /* ──────── Emit methods ──────── */

  function emitPlay(time: number) {
    send({ type: MSG_PLAYBACK_PLAY, data: { time } })
  }
  function emitPause(time: number) {
    send({ type: MSG_PLAYBACK_PAUSE, data: { time } })
  }
  function emitSeek(time: number) {
    send({ type: MSG_PLAYBACK_SEEK, data: { time } })
  }
  function emitTimeTick(time: number) {
    send({ type: MSG_PLAYBACK_TIME_TICK, data: { time } })
  }
  function emitChangeEpisode(episode_id: string) {
    send({ type: MSG_STATE_CHANGE_EPISODE, data: { episode_id } })
  }
  function emitChangePlayer(player: PlayerKind) {
    send({ type: MSG_STATE_CHANGE_PLAYER, data: { player } })
  }
  function emitChangeTranslation(translation_id: string) {
    send({ type: MSG_STATE_CHANGE_TRANSLATION, data: { translation_id } })
  }
  function sendChat(body: string) {
    const trimmed = body.trim()
    if (trimmed.length === 0) return
    if (body.length > CHAT_MAX_LEN) return
    send({ type: MSG_CHAT_MESSAGE, data: { body } })
  }
  function sendReaction(emoji: string) {
    if (!REACTION_SET.has(emoji)) return
    send({ type: MSG_CHAT_REACTION, data: { emoji } })
  }
  function heartbeat() {
    send({ type: MSG_PRESENCE_HEARTBEAT, data: {} })
  }

  /* ──────── Subscribe methods ──────── */

  // Generic subscribe-factory: adds the handler to the set, returns an
  // unsubscribe closure that removes only that handler.
  function subscribe<T>(set: Set<Handler<T>>) {
    return (handler: Handler<T>): (() => void) => {
      set.add(handler)
      return () => {
        set.delete(handler)
      }
    }
  }

  const onPlaybackEvent = subscribe(handlers.playbackEvent)
  const onStateChanged = subscribe(handlers.stateChanged)
  const onChatMessage = subscribe(handlers.chatMessage)
  const onReaction = subscribe(handlers.reaction)
  const onMemberJoined = subscribe(handlers.memberJoined)
  const onMemberLeft = subscribe(handlers.memberLeft)
  const onCorrection = subscribe(handlers.correction)
  const onError = subscribe(handlers.error)
  const onRoomClosed = subscribe(handlers.roomClosed)

  /* ──────── Vue lifecycle ──────── */

  // Auto-disconnect on component unmount. Skipped if the composable is
  // instantiated outside a setup context (e.g. unit tests, ad-hoc scripts).
  if (getCurrentInstance()) {
    onUnmounted(() => {
      disconnect()
    })
  }

  return {
    room,
    members,
    messages,
    reactions,
    connectionStatus,
    lastError,
    emitPlay,
    emitPause,
    emitSeek,
    emitTimeTick,
    emitChangeEpisode,
    emitChangePlayer,
    emitChangeTranslation,
    sendChat,
    sendReaction,
    heartbeat,
    onPlaybackEvent,
    onStateChanged,
    onChatMessage,
    onReaction,
    onMemberJoined,
    onMemberLeft,
    onCorrection,
    onError,
    onRoomClosed,
    connect,
    disconnect,
  }
}
