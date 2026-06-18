/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.1.
 *
 * TypeScript mirror of the Go domain for the watch-together service. Every
 * wire-string constant + payload interface lines up 1:1 with:
 *
 *   - services/watch-together/internal/domain/ws_message.go (envelope, 20 type
 *     constants, 7 error codes, 5 player IDs, 2 playback states, payload structs)
 *   - services/watch-together/internal/domain/room.go        (Room, MemberMeta, Member, RoomSnapshot)
 *   - services/watch-together/internal/domain/message.go     (ChatMessage)
 *   - services/watch-together/internal/service/inbound.go    (REACTION_WHITELIST)
 *
 * Conventions:
 *   - Snake_case JSON field names (matching Go `json` tags). Do NOT camelCase —
 *     `JSON.parse(...)` produces snake_case and the rest of the codebase keeps
 *     fields snake_case end-to-end per the spotlight workstream Pitfall 8.
 *   - String-literal unions (not enums) so wire strings can be passed directly
 *     to `JSON.stringify({type, data})`. TypeScript's structural typing makes
 *     enum<→string round-trips error-prone; literals avoid the issue.
 *   - Discriminated `Envelope<T>` generic. Callers narrow on `.type` to pick
 *     the right payload interface.
 *
 * Downstream plans (02.2 composable, 02.3 view, 02.4 chat panel, 02.5
 * reaction palette) import every symbol they need from this file directly —
 * the API client `@/api/watch-together` re-exports the public surface so a
 * single `import { ..., type ... } from '@/api/watch-together'` is the
 * canonical import for consumers.
 */

/* ──────────────────────────────────────────────────────────────────────── */
/*  Inbound message types (client → server). 10 types.                      */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:23-39.    */
/* ──────────────────────────────────────────────────────────────────────── */

/** Mirrors Go `MsgPlaybackPlay`. */
export const MSG_PLAYBACK_PLAY = 'playback:play' as const
/** Mirrors Go `MsgPlaybackPause`. */
export const MSG_PLAYBACK_PAUSE = 'playback:pause' as const
/** Mirrors Go `MsgPlaybackSeek`. Rate-limited 1/sec/user (WT-NF-02). */
export const MSG_PLAYBACK_SEEK = 'playback:seek' as const
/** Mirrors Go `MsgPlaybackTimeTick`. 1Hz drift heartbeat — never re-broadcast. */
export const MSG_PLAYBACK_TIME_TICK = 'playback:time_tick' as const

/** Mirrors Go `MsgStateChangeEpisode`. */
export const MSG_STATE_CHANGE_EPISODE = 'state:change_episode' as const
/** Mirrors Go `MsgStateChangePlayer`. */
export const MSG_STATE_CHANGE_PLAYER = 'state:change_player' as const
/** Mirrors Go `MsgStateChangeTrans`. */
export const MSG_STATE_CHANGE_TRANSLATION = 'state:change_translation' as const

/** Mirrors Go `MsgChatMessage`. Body ≤500 chars; over-cap → CHAT_TOO_LONG. */
export const MSG_CHAT_MESSAGE = 'chat:message' as const
/** Mirrors Go `MsgChatReaction`. Whitelist-checked; out-of-whitelist drops silently. */
export const MSG_CHAT_REACTION = 'chat:reaction' as const

/** Mirrors Go `MsgPresenceHeartbeat`. Empty payload; bumps server last_seen_at. */
export const MSG_PRESENCE_HEARTBEAT = 'presence:heartbeat' as const

/** Discriminator union for every inbound (client→server) envelope `.type`. */
export type MsgInbound =
  | typeof MSG_PLAYBACK_PLAY
  | typeof MSG_PLAYBACK_PAUSE
  | typeof MSG_PLAYBACK_SEEK
  | typeof MSG_PLAYBACK_TIME_TICK
  | typeof MSG_STATE_CHANGE_EPISODE
  | typeof MSG_STATE_CHANGE_PLAYER
  | typeof MSG_STATE_CHANGE_TRANSLATION
  | typeof MSG_CHAT_MESSAGE
  | typeof MSG_CHAT_REACTION
  | typeof MSG_PRESENCE_HEARTBEAT

/* ──────────────────────────────────────────────────────────────────────── */
/*  Outbound message types (server → client). 10 types.                     */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:46-57.    */
/* ──────────────────────────────────────────────────────────────────────── */

/** Mirrors Go `MsgRoomSnapshot`. Sent once per fresh WS connect/reconnect. */
export const MSG_ROOM_SNAPSHOT = 'room:snapshot' as const
/** Mirrors Go `MsgRoomStateChanged`. Single field+value mutation broadcast. */
export const MSG_ROOM_STATE_CHANGED = 'room:state_changed' as const
/** Mirrors Go `MsgPlaybackEvent`. play/pause/seek broadcast with attribution. */
export const MSG_PLAYBACK_EVENT = 'playback:event' as const
/** Mirrors Go `MsgPlaybackCorrection`. Per-recipient drift nudge — never broadcast. */
export const MSG_PLAYBACK_CORRECTION = 'playback:correction' as const
/** Mirrors Go `MsgMemberJoined`. Roster delta — full Member shipped. */
export const MSG_MEMBER_JOINED = 'member:joined' as const
/** Mirrors Go `MsgMemberLeft`. Roster delta — just user_id. */
export const MSG_MEMBER_LEFT = 'member:left' as const
/**
 * Mirrors Go `MsgChatMessageOut`. Wire string IDENTICAL to `MSG_CHAT_MESSAGE`
 * by design — the inbound and outbound `chat:message` share a dispatch token
 * but carry different payload shapes (sender-supplied body vs server-attributed
 * full ChatMessage).
 */
export const MSG_CHAT_MESSAGE_OUT = 'chat:message' as const
/** Mirrors Go `MsgChatReactionOut`. Wire string identical to `MSG_CHAT_REACTION`. */
export const MSG_CHAT_REACTION_OUT = 'chat:reaction' as const
/** Mirrors Go `MsgRoomClosed`. Final outbound before WS close. */
export const MSG_ROOM_CLOSED = 'room:closed' as const
/** Mirrors Go `MsgError`. Universal error envelope. Sender-only. */
export const MSG_ERROR = 'error' as const

/** Discriminator union for every outbound (server→client) envelope `.type`. */
export type MsgOutbound =
  | typeof MSG_ROOM_SNAPSHOT
  | typeof MSG_ROOM_STATE_CHANGED
  | typeof MSG_PLAYBACK_EVENT
  | typeof MSG_PLAYBACK_CORRECTION
  | typeof MSG_MEMBER_JOINED
  | typeof MSG_MEMBER_LEFT
  | typeof MSG_CHAT_MESSAGE_OUT
  | typeof MSG_CHAT_REACTION_OUT
  | typeof MSG_ROOM_CLOSED
  | typeof MSG_ERROR

/* ──────────────────────────────────────────────────────────────────────── */
/*  Error codes — 7 strings carried by ErrorData.code.                      */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:64-71.    */
/* ──────────────────────────────────────────────────────────────────────── */

export const ERR_CAPACITY_FULL = 'CAPACITY_FULL' as const
export const ERR_ROOM_NOT_FOUND = 'ROOM_NOT_FOUND' as const
export const ERR_RATE_LIMITED = 'RATE_LIMITED' as const
export const ERR_CHAT_TOO_LONG = 'CHAT_TOO_LONG' as const
export const ERR_PERSISTENT_DRIFT = 'PERSISTENT_DRIFT' as const
export const ERR_AUTH_EXPIRED = 'AUTH_EXPIRED' as const
export const ERR_EPISODE_UNAVAILABLE = 'EPISODE_UNAVAILABLE' as const
/** Phase 04 state-switching: sender-only error when the requested player has
 *  no episodes for this anime. Mirrors Go `ErrPlayerUnavailable`. */
export const ERR_PLAYER_UNAVAILABLE = 'PLAYER_UNAVAILABLE' as const
/** Phase 04 state-switching: sender-only error when the requested translation
 *  is not available for the (anime, player, episode) tuple. Mirrors Go
 *  `ErrTranslationUnavailable`. */
export const ERR_TRANSLATION_UNAVAILABLE = 'TRANSLATION_UNAVAILABLE' as const

/** All nine server-emitted error codes (7 from Phase 1 + 2 from Phase 4). */
export type ErrorCode =
  | typeof ERR_CAPACITY_FULL
  | typeof ERR_ROOM_NOT_FOUND
  | typeof ERR_RATE_LIMITED
  | typeof ERR_CHAT_TOO_LONG
  | typeof ERR_PERSISTENT_DRIFT
  | typeof ERR_AUTH_EXPIRED
  | typeof ERR_EPISODE_UNAVAILABLE
  | typeof ERR_PLAYER_UNAVAILABLE
  | typeof ERR_TRANSLATION_UNAVAILABLE

/* ──────────────────────────────────────────────────────────────────────── */
/*  Player + playback-state unions.                                         */
/*  Mirrors ws_message.go:79-91. Five players in the user-facing player     */
/*  matrix; two playback states.                                            */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * The five frontend player IDs accepted by `Room.player`. Matches the five
 * `<*Player>` Vue components in `frontend/web/src/components/player/`.
 */
export type PlayerKind = 'kodik' | 'kodik-adfree' | 'animelib' | 'ourenglish' | 'hanime' | 'raw' | 'aeplayer'

/** Playback state union — either the video is `playing` or `paused`. */
export type PlaybackState = 'playing' | 'paused'

/* ──────────────────────────────────────────────────────────────────────── */
/*  Protocol version + reaction whitelist.                                  */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Mirrors `domain.ProtocolVersion` (`ws_message.go:97`). Sent on every
 * `room:snapshot`; frontend rejects connections whose snapshot's
 * `protocol_version` doesn't match.
 */
export const PROTOCOL_VERSION = '1.0' as const

/**
 * The 24 anime-friendly emoji accepted by `chat:reaction`. Locked to match
 * the Go map in `services/watch-together/internal/service/inbound.go:92-117`
 * VERBATIM. Reactions outside this set are silently dropped by the server.
 *
 * Codepoint notes:
 *   - `❤️` is U+2764 RED HEART + U+FE0F VARIATION SELECTOR-16. The VS16
 *     "dresses" the heart as an emoji (vs the text-presentation default).
 *     Most iOS / Android / Slack pickers emit it with the VS16; matching
 *     the bare U+2764 would silently fail at the server.
 *   - `⚡` is U+26A1 HIGH VOLTAGE SIGN.
 *
 * Type is `readonly [...] as const` so consumers can narrow against the
 * exact literal tuple. Used by Plan 02.5 ReactionPalette + the inbound
 * spec contract.
 */
export const REACTION_WHITELIST = [
  '🔥',
  '❤️',
  '😂',
  '😭',
  '👀',
  '🙏',
  '🎉',
  '✨',
  '💀',
  '🥺',
  '😍',
  '🤔',
  '👏',
  '🙌',
  '😱',
  '😎',
  '🌸',
  '⚡',
  '💯',
  '🎌',
  '🍣',
  '🌟',
  '💢',
  '🤯',
] as const

/** Element type of `REACTION_WHITELIST`. */
export type ReactionEmoji = (typeof REACTION_WHITELIST)[number]

/* ──────────────────────────────────────────────────────────────────────── */
/*  Core domain types.                                                      */
/*  Mirrors services/watch-together/internal/domain/room.go +               */
/*           services/watch-together/internal/domain/message.go.            */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Mirrors `domain.Room` (room.go:14-27). Canonical Redis HASH for a room.
 * `playback_time_updated_at` is unix milliseconds — used as the drift-
 * detection anchor (see DriftEngine.OnTimeTick).
 */
export interface Room {
  id: string
  created_at: number // unix seconds
  anime_id: string
  episode_id: string
  player: PlayerKind
  translation_id: string
  playback_state: PlaybackState
  playback_time: number // seconds into the episode
  playback_time_updated_at: number // unix milliseconds (drift anchor)
  host_user_id: string // cosmetic only — any member can drive playback
}

/**
 * Mirrors `domain.MemberMeta` (room.go:31-36). Value half of the
 * `wt:room:{roomId}:members` Redis HASH; key is the user ID.
 */
export interface MemberMeta {
  username: string
  avatar_url: string
  joined_at: number // unix seconds
  last_seen_at: number // unix seconds — bumped on presence:heartbeat
}

/**
 * Mirrors `domain.Member` (room.go:40-43). On-the-wire pairing of user_id
 * + MemberMeta. Used in `RoomSnapshot.members` and `member:joined` payloads.
 */
export interface Member {
  user_id: string
  meta: MemberMeta
}

/**
 * Mirrors `domain.ChatMessage` (message.go:9-15). Persisted entry in
 * `wt:room:{roomId}:messages` (Redis LIST, capped at 100). `ts` is unix
 * milliseconds — chosen for ordering granularity finer than seconds, since
 * a chatty room can produce >1 msg/sec.
 */
export interface ChatMessage {
  id: string
  user_id: string
  username: string
  body: string
  ts: number // unix milliseconds
}

/**
 * Mirrors `domain.RoomSnapshot` (room.go:50-55). Returned by the outbound
 * `room:snapshot` envelope on every WS connect/reconnect; also the JSON
 * body of `GET /api/watch-together/rooms/{id}`.
 *
 * `messages` is the most recent 50 entries (the LIST holds up to 100 but
 * the snapshot caps retrieval). `protocol_version` must equal
 * `PROTOCOL_VERSION` — incompatible servers are rejected client-side.
 */
export interface RoomSnapshot {
  room: Room
  members: Member[]
  messages: ChatMessage[]
  protocol_version: string
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Inbound payload interfaces (client → server).                           */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:104-153.  */
/* ──────────────────────────────────────────────────────────────────────── */

/** Mirrors Go `PlaybackPlayData`. `time` = seconds into the episode. */
export interface PlaybackPlayData {
  time: number
}

/** Mirrors Go `PlaybackPauseData`. Same shape as PlaybackPlayData (intentional). */
export interface PlaybackPauseData {
  time: number
}

/** Mirrors Go `PlaybackSeekData`. Same shape (intentional — see Go file comment). */
export interface PlaybackSeekData {
  time: number
}

/** Mirrors Go `PlaybackTimeTickData`. 1Hz client-reported wall-clock. */
export interface PlaybackTimeTickData {
  time: number
}

/** Mirrors Go `StateChangeEpisodeData`. */
export interface StateChangeEpisodeData {
  episode_id: string
}

/** Mirrors Go `StateChangePlayerData`. `player` is one of `PlayerKind`. */
export interface StateChangePlayerData {
  player: PlayerKind
}

/** Mirrors Go `StateChangeTranslationData`. */
export interface StateChangeTranslationData {
  translation_id: string
}

/**
 * Mirrors Go `ChatMessageInData`. Body ≤500 chars; the handler rejects
 * over-cap with an `ERR_CHAT_TOO_LONG` sender-only error envelope (NOT
 * truncated silently).
 */
export interface ChatMessageInData {
  body: string
}

/**
 * Mirrors Go `ChatReactionInData`. Single emoji from `REACTION_WHITELIST`;
 * non-whitelist emoji are silently dropped server-side.
 */
export interface ChatReactionInData {
  emoji: string
}

/**
 * Mirrors Go `PresenceHeartbeatData{}` — intentionally empty. The act of
 * sending it bumps the sender's `last_seen_at` server-side. Typed as
 * `Record<string, never>` so TS rejects any accidental fields.
 */
export type PresenceHeartbeatData = Record<string, never>

/* ──────────────────────────────────────────────────────────────────────── */
/*  Outbound payload interfaces (server → client).                          */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:155-230.  */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Mirrors Go `RoomStateChangedData`. `field` is the JSON tag on `Room`
 * (e.g. `"player"`, `"translation_id"`, `"episode_id"`); `value` is the
 * new value (typed `unknown` here since the server's interface{} can
 * carry strings or numbers depending on the field).
 */
export interface RoomStateChangedData {
  field: string
  value: unknown
  by_user_id: string
}

/**
 * Mirrors Go `PlaybackEventData`. `kind` ∈ `'play' | 'pause' | 'seek'`.
 * `server_ts` is unix ms — the authoritative wall-clock the client uses
 * as the drift-detection anchor on next time_tick.
 */
export interface PlaybackEventData {
  kind: 'play' | 'pause' | 'seek'
  time: number
  by_user_id: string
  server_ts: number // unix milliseconds
}

/**
 * Mirrors Go `PlaybackCorrectionData`. Per-recipient drift nudge — the
 * client decides nudge (soft) vs hard-seek (hard) based on observed
 * drift magnitude. After 5 consecutive corrections that exceed threshold,
 * the server stops correcting and sends an `ERR_PERSISTENT_DRIFT` error
 * envelope.
 */
export interface PlaybackCorrectionData {
  time: number
  server_ts: number // unix milliseconds
}

/**
 * Mirrors Go `MemberJoinedData`. Full `MemberMeta` is shipped so the
 * receiver can update its roster without a separate roster fetch.
 */
export interface MemberJoinedData {
  user_id: string
  member: MemberMeta
}

/**
 * Mirrors Go `MemberLeftData`. Just `user_id` — receivers already have
 * the meta from the matching `member:joined` event.
 */
export interface MemberLeftData {
  user_id: string
}

/**
 * Mirrors Go `ChatMessageOutData`. Wraps a server-attributed `ChatMessage`
 * (with server-generated `id`, server-stamped `ts`, sender `user_id` +
 * `username`).
 */
export interface ChatMessageOutData {
  message: ChatMessage
}

/**
 * Mirrors Go `ChatReactionOutData`. NOT persisted server-side — ephemeral
 * burst purely for the floating-emoji overlay UI (3s fade is client-side).
 */
export interface ChatReactionOutData {
  user_id: string
  emoji: string
}

/**
 * Mirrors Go `RoomClosedData`. Last outbound before WS close. `reason` is
 * a short machine-readable token (e.g. `"host_closed"`,
 * `"grace_period_expired"`).
 */
export interface RoomClosedData {
  reason: string
}

/**
 * Mirrors Go `ErrorData`. Universal sender-only error envelope payload.
 * `code` is one of the `ERR_*` constants; `message` is human-readable
 * (optional); `hint` suggests a recovery action (e.g. `"reload"` on
 * `ERR_PERSISTENT_DRIFT`).
 */
export interface ErrorData {
  code: ErrorCode | string
  message?: string
  hint?: string
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Envelope generic.                                                       */
/*  Mirrors services/watch-together/internal/domain/ws_message.go:10-13.    */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Universal WebSocket envelope. `type` discriminates the payload schema;
 * `data` is the typed body shape. The generic parameter `T` lets callers
 * either pin a specific payload (`Envelope<ChatMessageInData>`) or accept
 * the typed-union shorthand (`InboundEnvelope` / `OutboundEnvelope` below).
 *
 * Wire format: `{"type":"chat:message","data":{"body":"hi"}}`.
 */
export interface Envelope<T = unknown> {
  type: string
  data: T
}

/**
 * Discriminated union of every outbound envelope shape. Use in a `switch`
 * on `.type` to get exhaustive payload narrowing — the compiler will flag
 * any missed branch.
 */
export type OutboundEnvelope =
  | Envelope<RoomSnapshot> & { type: typeof MSG_ROOM_SNAPSHOT }
  | Envelope<RoomStateChangedData> & { type: typeof MSG_ROOM_STATE_CHANGED }
  | Envelope<PlaybackEventData> & { type: typeof MSG_PLAYBACK_EVENT }
  | Envelope<PlaybackCorrectionData> & { type: typeof MSG_PLAYBACK_CORRECTION }
  | Envelope<MemberJoinedData> & { type: typeof MSG_MEMBER_JOINED }
  | Envelope<MemberLeftData> & { type: typeof MSG_MEMBER_LEFT }
  | Envelope<ChatMessageOutData> & { type: typeof MSG_CHAT_MESSAGE_OUT }
  | Envelope<ChatReactionOutData> & { type: typeof MSG_CHAT_REACTION_OUT }
  | Envelope<RoomClosedData> & { type: typeof MSG_ROOM_CLOSED }
  | Envelope<ErrorData> & { type: typeof MSG_ERROR }

/**
 * Discriminated union of every inbound envelope shape. Mirrors the
 * dispatch table in `services/watch-together/internal/service/inbound.go`
 * (lines 186-214).
 *
 * NOTE: `MSG_CHAT_MESSAGE` / `MSG_CHAT_REACTION` share their wire strings
 * with their outbound siblings (`MSG_CHAT_MESSAGE_OUT` / `MSG_CHAT_REACTION_OUT`).
 * The inbound vs outbound distinction is enforced by the payload shape only,
 * not by `.type`.
 */
export type InboundEnvelope =
  | Envelope<PlaybackPlayData> & { type: typeof MSG_PLAYBACK_PLAY }
  | Envelope<PlaybackPauseData> & { type: typeof MSG_PLAYBACK_PAUSE }
  | Envelope<PlaybackSeekData> & { type: typeof MSG_PLAYBACK_SEEK }
  | Envelope<PlaybackTimeTickData> & { type: typeof MSG_PLAYBACK_TIME_TICK }
  | Envelope<StateChangeEpisodeData> & { type: typeof MSG_STATE_CHANGE_EPISODE }
  | Envelope<StateChangePlayerData> & { type: typeof MSG_STATE_CHANGE_PLAYER }
  | Envelope<StateChangeTranslationData> & { type: typeof MSG_STATE_CHANGE_TRANSLATION }
  | Envelope<ChatMessageInData> & { type: typeof MSG_CHAT_MESSAGE }
  | Envelope<ChatReactionInData> & { type: typeof MSG_CHAT_REACTION }
  | Envelope<PresenceHeartbeatData> & { type: typeof MSG_PRESENCE_HEARTBEAT }

/* ──────────────────────────────────────────────────────────────────────── */
/*  REST shapes — POST/GET/DELETE /api/watch-together/rooms[/{id}].         */
/*  Mirrors services/watch-together/internal/handler/rooms.go:50-66.        */
/* ──────────────────────────────────────────────────────────────────────── */

/** JSON body for POST /api/watch-together/rooms. Mirrors Go `CreateRoomBody`. */
export interface CreateRoomRequest {
  anime_id: string
  episode_id: string
  player: PlayerKind
  translation_id: string
}

/**
 * JSON response from POST /api/watch-together/rooms. Mirrors Go
 * `CreateRoomResponse`. The frontend uses `room_id` to navigate to
 * `/watch/room/{room_id}`, copies `invite_url` to clipboard, and (later,
 * in Phase 2 composable) opens the WS at `ws_url`.
 */
export interface CreateRoomResponse {
  room_id: string
  invite_url: string
  ws_url: string
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Custom Error subclasses — used by the API client to distinguish        */
/*  status-code-specific failures from generic axios errors.               */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * Thrown by `getRoom(id)` / `deleteRoom(id)` when the server returns 410
 * (room expired or doesn't exist). Callers should render the "room ended"
 * empty state with a "back to anime" button.
 *
 * Subclass is `Error` (not `AxiosError`) so the caller doesn't need to
 * import axios types — `err instanceof RoomGoneError` is the contract.
 */
export class RoomGoneError extends Error {
  constructor(message = 'room expired or does not exist') {
    super(message)
    this.name = 'RoomGoneError'
    // ES5 prototype chain repair — ensures `instanceof RoomGoneError`
    // works after the class is transpiled.
    Object.setPrototypeOf(this, RoomGoneError.prototype)
  }
}

/**
 * Thrown by `deleteRoom(id)` when the server returns 403 (caller is not
 * the host — `WT-FOUND-03`). Only the room's `host_user_id` can force-close.
 */
export class RoomForbiddenError extends Error {
  constructor(message = 'only the host can delete this room') {
    super(message)
    this.name = 'RoomForbiddenError'
    Object.setPrototypeOf(this, RoomForbiddenError.prototype)
  }
}
