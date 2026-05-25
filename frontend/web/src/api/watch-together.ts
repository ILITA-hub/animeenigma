/**
 * Typed API client for the watch-together service.
 *
 * Routes (gateway-proxied; JWT-required at the gateway):
 *   POST   /api/watch-together/rooms              → CreateRoomResponse
 *   GET    /api/watch-together/rooms/{id}         → RoomSnapshot
 *   DELETE /api/watch-together/rooms/{id}         → 204 (host only — 403 if not)
 *
 * Backend wraps every JSON response in libs/httputil's `{success, data}`
 * envelope; `unwrap<T>(...)` strips it. Status-code branching is local to
 * the two callers that need it — `getRoom` distinguishes 410 (room expired),
 * `deleteRoom` distinguishes 403 (not host) and 410. All other errors
 * propagate verbatim (axios.AxiosError instances).
 *
 * Workstream: watch-together, Phase 2 (frontend-shell), Plan 02.1.
 */

import axios from 'axios'

import { apiClient } from '@/api/client'
import {
  RoomGoneError,
  RoomForbiddenError,
  type CreateRoomRequest,
  type CreateRoomResponse,
  type RoomSnapshot,
} from '@/types/watch-together'

/**
 * Unwrap the standard `{success, data}` envelope. Backends sometimes return
 * the bare payload when the envelope helper is bypassed (tests, internal
 * callers); the fallback `?? raw` keeps the helper robust against either
 * shape. Copy-pasted from `notifications.ts:34-42` per the project pattern
 * of co-locating tiny helpers rather than extracting to a shared util.
 */
function unwrap<T>(raw: unknown): T {
  if (raw && typeof raw === 'object' && 'data' in (raw as Record<string, unknown>)) {
    const data = (raw as { data?: unknown }).data
    if (data !== undefined && data !== null) {
      return data as T
    }
  }
  return raw as T
}

/**
 * POST /api/watch-together/rooms
 *
 * Creates a new room and returns the trio of identifiers the frontend
 * needs to land on the room view + open the WS:
 *   - `room_id` — used to navigate to `/watch/room/:roomId`
 *   - `invite_url` — copied to clipboard for sharing
 *   - `ws_url` — passed to `new WebSocket(...)` by the composable (Plan 02.2)
 *
 * Validation errors (missing/blank fields) come back as 400 with the
 * standard error envelope; the caller surfaces them via toast.
 */
export async function createRoom(payload: CreateRoomRequest): Promise<CreateRoomResponse> {
  const response = await apiClient.post('/watch-together/rooms', payload)
  return unwrap<CreateRoomResponse>(response.data)
}

/**
 * GET /api/watch-together/rooms/{id}
 *
 * Returns the canonical `RoomSnapshot` (room state + members + last 50
 * messages + protocol_version) for a live room. 410 Gone means the room's
 * TTL has expired or the host force-closed it; the caller renders the
 * "room ended" empty state.
 *
 * URL-encodes `id` to handle pathological inputs (slashes, spaces). Room
 * IDs from `createRoom` are URL-safe UUIDs in practice, but a forged or
 * fuzzed input shouldn't break path routing.
 */
export async function getRoom(id: string): Promise<RoomSnapshot> {
  try {
    const response = await apiClient.get(`/watch-together/rooms/${encodeURIComponent(id)}`)
    return unwrap<RoomSnapshot>(response.data)
  } catch (err) {
    if (axios.isAxiosError(err) && err.response?.status === 410) {
      throw new RoomGoneError()
    }
    throw err
  }
}

/**
 * DELETE /api/watch-together/rooms/{id}
 *
 * Host-only force-close (`WT-FOUND-03`). Resolves to undefined on 204.
 *
 * Status-code branching:
 *   - 403 → `RoomForbiddenError` (caller is not the host)
 *   - 410 → `RoomGoneError` (room already gone — same UX as get 410)
 *   - other → rethrown verbatim
 */
export async function deleteRoom(id: string): Promise<void> {
  try {
    await apiClient.delete(`/watch-together/rooms/${encodeURIComponent(id)}`)
  } catch (err) {
    if (axios.isAxiosError(err)) {
      if (err.response?.status === 403) {
        throw new RoomForbiddenError()
      }
      if (err.response?.status === 410) {
        throw new RoomGoneError()
      }
    }
    throw err
  }
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Re-exports — single canonical import path for consumers.                */
/*                                                                          */
/*  Downstream plans (02.2 composable, 02.3 view, etc.) import from         */
/*  `@/api/watch-together` so a single path covers both the type symbols    */
/*  and the API surface. Matches the spotlight workstream pattern (the      */
/*  composable consumers import types via `@/types/spotlight`, but for     */
/*  watch-together we co-locate so the SUT and its types live behind one   */
/*  import line for ergonomics).                                            */
/* ──────────────────────────────────────────────────────────────────────── */

export {
  // Errors — needed by the composable to branch on rejection reason.
  RoomGoneError,
  RoomForbiddenError,
} from '@/types/watch-together'

export type {
  // Core domain
  Room,
  Member,
  MemberMeta,
  ChatMessage,
  RoomSnapshot,
  // Wire envelope
  Envelope,
  InboundEnvelope,
  OutboundEnvelope,
  // REST shapes
  CreateRoomRequest,
  CreateRoomResponse,
  // Unions
  PlayerKind,
  PlaybackState,
  ErrorCode,
  MsgInbound,
  MsgOutbound,
  ReactionEmoji,
  // Inbound payloads
  PlaybackPlayData,
  PlaybackPauseData,
  PlaybackSeekData,
  PlaybackTimeTickData,
  StateChangeEpisodeData,
  StateChangePlayerData,
  StateChangeTranslationData,
  ChatMessageInData,
  ChatReactionInData,
  PresenceHeartbeatData,
  // Outbound payloads
  RoomStateChangedData,
  PlaybackEventData,
  PlaybackCorrectionData,
  MemberJoinedData,
  MemberLeftData,
  ChatMessageOutData,
  ChatReactionOutData,
  RoomClosedData,
  ErrorData,
} from '@/types/watch-together'

export {
  // Inbound type constants
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
  // Outbound type constants
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
  // Error codes
  ERR_CAPACITY_FULL,
  ERR_ROOM_NOT_FOUND,
  ERR_RATE_LIMITED,
  ERR_CHAT_TOO_LONG,
  ERR_PERSISTENT_DRIFT,
  ERR_AUTH_EXPIRED,
  ERR_EPISODE_UNAVAILABLE,
  ERR_PLAYER_UNAVAILABLE,
  ERR_TRANSLATION_UNAVAILABLE,
  // Versioning + content
  PROTOCOL_VERSION,
  REACTION_WHITELIST,
} from '@/types/watch-together'
