/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.1 / Task 1.
 *
 * Spec for `frontend/web/src/types/watch-together.ts`. Locks the wire-string
 * constants character-for-character against the Go source
 * (`services/watch-together/internal/domain/ws_message.go`) so a future drift
 * trips this test instead of silently breaking the WS protocol.
 *
 * Also asserts the 24-emoji `REACTION_WHITELIST` matches the Go
 * `reactionWhitelist` map in `services/watch-together/internal/service/inbound.go`
 * — the frontend palette in Plan 02.5 binds against this same constant.
 */

import { describe, it, expect } from 'vitest'

import {
  // Inbound message types (10)
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
  // Outbound message types (10)
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
  // Error codes (7)
  ERR_CAPACITY_FULL,
  ERR_ROOM_NOT_FOUND,
  ERR_RATE_LIMITED,
  ERR_CHAT_TOO_LONG,
  ERR_PERSISTENT_DRIFT,
  ERR_AUTH_EXPIRED,
  ERR_EPISODE_UNAVAILABLE,
  // Other constants
  PROTOCOL_VERSION,
  REACTION_WHITELIST,
  // Custom errors
  RoomGoneError,
  RoomForbiddenError,
  // Type-only — pulled in for the satisfies-check
  type PlayerKind,
} from '@/types/watch-together'

describe('watch-together type constants — inbound wire strings', () => {
  it('matches all 10 inbound message types char-for-char with Go domain', () => {
    expect(MSG_PLAYBACK_PLAY).toBe('playback:play')
    expect(MSG_PLAYBACK_PAUSE).toBe('playback:pause')
    expect(MSG_PLAYBACK_SEEK).toBe('playback:seek')
    expect(MSG_PLAYBACK_TIME_TICK).toBe('playback:time_tick')
    expect(MSG_STATE_CHANGE_EPISODE).toBe('state:change_episode')
    expect(MSG_STATE_CHANGE_PLAYER).toBe('state:change_player')
    expect(MSG_STATE_CHANGE_TRANSLATION).toBe('state:change_translation')
    expect(MSG_CHAT_MESSAGE).toBe('chat:message')
    expect(MSG_CHAT_REACTION).toBe('chat:reaction')
    expect(MSG_PRESENCE_HEARTBEAT).toBe('presence:heartbeat')
  })
})

describe('watch-together type constants — outbound wire strings', () => {
  it('matches all 10 outbound message types char-for-char with Go domain', () => {
    expect(MSG_ROOM_SNAPSHOT).toBe('room:snapshot')
    expect(MSG_ROOM_STATE_CHANGED).toBe('room:state_changed')
    expect(MSG_PLAYBACK_EVENT).toBe('playback:event')
    expect(MSG_PLAYBACK_CORRECTION).toBe('playback:correction')
    expect(MSG_MEMBER_JOINED).toBe('member:joined')
    expect(MSG_MEMBER_LEFT).toBe('member:left')
    // chat:message and chat:reaction share wire strings with their inbound
    // siblings by design — different payload shape, same dispatch token.
    expect(MSG_CHAT_MESSAGE_OUT).toBe('chat:message')
    expect(MSG_CHAT_REACTION_OUT).toBe('chat:reaction')
    expect(MSG_ROOM_CLOSED).toBe('room:closed')
    expect(MSG_ERROR).toBe('error')
  })

  it('chat in/out wire strings are the same string (by design)', () => {
    expect(MSG_CHAT_MESSAGE_OUT).toBe(MSG_CHAT_MESSAGE)
    expect(MSG_CHAT_REACTION_OUT).toBe(MSG_CHAT_REACTION)
  })
})

describe('watch-together error codes', () => {
  it('matches all 7 error code strings from Go ErrCode*', () => {
    expect(ERR_CAPACITY_FULL).toBe('CAPACITY_FULL')
    expect(ERR_ROOM_NOT_FOUND).toBe('ROOM_NOT_FOUND')
    expect(ERR_RATE_LIMITED).toBe('RATE_LIMITED')
    expect(ERR_CHAT_TOO_LONG).toBe('CHAT_TOO_LONG')
    expect(ERR_PERSISTENT_DRIFT).toBe('PERSISTENT_DRIFT')
    expect(ERR_AUTH_EXPIRED).toBe('AUTH_EXPIRED')
    expect(ERR_EPISODE_UNAVAILABLE).toBe('EPISODE_UNAVAILABLE')
  })
})

describe('watch-together protocol version', () => {
  it('exposes PROTOCOL_VERSION = "1.0"', () => {
    expect(PROTOCOL_VERSION).toBe('1.0')
  })
})

describe('watch-together REACTION_WHITELIST', () => {
  it('is exactly 24 emoji', () => {
    expect(REACTION_WHITELIST).toHaveLength(24)
  })

  it('includes the three canonical anchors (with U+FE0F + U+26A1 preserved)', () => {
    // ❤️ is U+2764 U+FE0F — the dressed variation selector is required to
    // match what most clients (iOS, Android, Slack) emit. Stripping the VS16
    // would silently break the whitelist match on the wire.
    expect(REACTION_WHITELIST).toContain('❤️')
    expect(REACTION_WHITELIST).toContain('🔥')
    expect(REACTION_WHITELIST).toContain('⚡') // U+26A1
  })

  it('every entry is unique', () => {
    expect(new Set(REACTION_WHITELIST).size).toBe(REACTION_WHITELIST.length)
  })

  it('matches the full Go reactionWhitelist set verbatim', () => {
    // Mirror of services/watch-together/internal/service/inbound.go:92-117.
    // Order is not significant; we compare as a set.
    const expected = new Set([
      '🔥', '❤️', '😂', '😭', '👀', '🙏', '🎉', '✨',
      '💀', '🥺', '😍', '🤔', '👏', '🙌', '😱', '😎',
      '🌸', '⚡', '💯', '🎌', '🍣', '🌟', '💢', '🤯',
    ])
    expect(new Set(REACTION_WHITELIST)).toEqual(expected)
  })
})

describe('watch-together PlayerKind', () => {
  it('admits exactly the 5 frontend player IDs at the type level', () => {
    // Runtime tuple that MUST type-check against PlayerKind[] — if a string
    // is added/removed/typo'd in the union, this stops compiling and the
    // tsc step in the verify gate trips.
    const all = ['kodik', 'kodik-adfree', 'animelib', 'ourenglish', 'hanime'] as const satisfies readonly PlayerKind[]
    expect(all).toHaveLength(5)
  })
})

describe('watch-together error subclasses', () => {
  it('RoomGoneError is an Error with the expected name', () => {
    const err = new RoomGoneError('room expired')
    expect(err).toBeInstanceOf(Error)
    expect(err).toBeInstanceOf(RoomGoneError)
    expect(err.name).toBe('RoomGoneError')
    expect(err.message).toBe('room expired')
  })

  it('RoomForbiddenError is an Error with the expected name', () => {
    const err = new RoomForbiddenError('not host')
    expect(err).toBeInstanceOf(Error)
    expect(err).toBeInstanceOf(RoomForbiddenError)
    expect(err.name).toBe('RoomForbiddenError')
    expect(err.message).toBe('not host')
  })
})
