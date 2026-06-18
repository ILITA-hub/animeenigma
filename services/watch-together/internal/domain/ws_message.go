package domain

import "encoding/json"

// Envelope is the universal frame for every WebSocket message in both
// directions. Type discriminates the payload schema (Msg* constants in this
// file); Data is the JSON-encoded body shaped per direction.
//
// Wire format example: `{"type":"chat:message","data":{"body":"hi"}}`.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ----------------------------------------------------------------------------
// Message type constants — inbound (client → server).
// 10 types per docs/superpowers/specs/2026-05-25-watch-together-design.md
// §WebSocket Protocol. Constants MUST NOT change once shipped — downstream
// plans (01.3 hub, 01.5 ws-handler) build against these identifiers.
// ----------------------------------------------------------------------------
const (
	// Playback control. server-broadcasts as playback:event with attribution.
	MsgPlaybackPlay     = "playback:play"
	MsgPlaybackPause    = "playback:pause"
	MsgPlaybackSeek     = "playback:seek"
	MsgPlaybackTimeTick = "playback:time_tick" // 1Hz heartbeat — NOT rebroadcast; drives drift detection

	// State mutations. Trigger room:state_changed broadcasts.
	MsgStateChangeEpisode = "state:change_episode"
	MsgStateChangePlayer  = "state:change_player"
	MsgStateChangeTrans   = "state:change_translation"

	// Chat / reactions.
	MsgChatMessage  = "chat:message"
	MsgChatReaction = "chat:reaction"

	// Presence.
	MsgPresenceHeartbeat = "presence:heartbeat"
)

// ----------------------------------------------------------------------------
// Message type constants — outbound (server → client).
// `chat:message` and `chat:reaction` use the same wire string in both
// directions but differ in payload (sender-supplied vs server-attributed).
// ----------------------------------------------------------------------------
const (
	MsgRoomSnapshot       = "room:snapshot"
	MsgRoomStateChanged   = "room:state_changed"
	MsgPlaybackEvent      = "playback:event"
	MsgPlaybackCorrection = "playback:correction" // per-recipient, not broadcast
	MsgMemberJoined       = "member:joined"
	MsgMemberLeft         = "member:left"
	MsgChatMessageOut     = "chat:message"  // shares the wire string with MsgChatMessage by design
	MsgChatReactionOut    = "chat:reaction" // shares the wire string with MsgChatReaction by design
	MsgRoomClosed         = "room:closed"
	MsgError              = "error"
)

// ----------------------------------------------------------------------------
// Error codes carried by ErrorData.Code. Strings (not ints) so the wire
// format is debuggable in wscat / browser devtools.
// ----------------------------------------------------------------------------
const (
	ErrCodeCapacityFull        = "CAPACITY_FULL"
	ErrCodeRoomNotFound        = "ROOM_NOT_FOUND"
	ErrCodeRateLimited         = "RATE_LIMITED"
	ErrCodeChatTooLong         = "CHAT_TOO_LONG"
	ErrCodePersistentDrift     = "PERSISTENT_DRIFT"
	ErrCodeAuthExpired         = "AUTH_EXPIRED"
	ErrCodeEpisodeUnavailable     = "EPISODE_UNAVAILABLE"
	ErrCodePlayerUnavailable      = "PLAYER_UNAVAILABLE"      // Phase 4 WT-STATE-02 — sent sender-only when state:change_player references a player with no episodes for the room's anime.
	ErrCodeTranslationUnavailable = "TRANSLATION_UNAVAILABLE" // Phase 4 WT-STATE-02 — sent sender-only when state:change_translation references a translation that yields no episodes for the room's anime+player.
)

// ----------------------------------------------------------------------------
// Player constants — string union for Room.Player. Mirror the 5 frontend
// player components (Kodik / AnimeLib / OurEnglish / Hanime / Raw — see
// CLAUDE.md §Video Player Architecture).
// ----------------------------------------------------------------------------
const (
	PlayerKodik      = "kodik"
	PlayerAnimeLib   = "animelib"
	PlayerOurEnglish = "ourenglish"
	PlayerHanime     = "hanime"
	PlayerRaw        = "raw"
	// PlayerAePlayer is the first-party AnimeEnigma unified player (multi-source).
	PlayerAePlayer = "aeplayer"
)

// ----------------------------------------------------------------------------
// Playback state constants — string union for Room.PlaybackState.
// ----------------------------------------------------------------------------
const (
	StatePlaying = "playing"
	StatePaused  = "paused"
)

// ProtocolVersion is sent on every `room:snapshot` so frontend clients can
// reject incompatible servers (forward-compat hook; see design doc
// §WebSocket protocol versioning).
const ProtocolVersion = "1.0"

// ----------------------------------------------------------------------------
// Inbound payload shapes (client → server). Handlers in plan 01.5 deserialize
// Envelope.Data into the matching struct based on Envelope.Type.
// ----------------------------------------------------------------------------

// PlaybackPlayData / PlaybackPauseData / PlaybackSeekData / PlaybackTimeTickData
// all carry just `time` (seconds into the episode). Kept as separate types so
// the inbound switch statement can use type-specific decoding even though the
// wire shape is identical today — protects against accidental coupling if any
// of them grows a field later.
type PlaybackPlayData struct {
	Time float64 `json:"time"`
}

type PlaybackPauseData struct {
	Time float64 `json:"time"`
}

type PlaybackSeekData struct {
	Time float64 `json:"time"`
}

type PlaybackTimeTickData struct {
	Time float64 `json:"time"`
}

// StateChangeEpisodeData / StateChangePlayerData / StateChangeTranslationData
// — one field per change kind, no further structure.
type StateChangeEpisodeData struct {
	EpisodeID string `json:"episode_id"`
}

type StateChangePlayerData struct {
	Player string `json:"player"` // PlayerKodik | PlayerAnimeLib | ...
}

type StateChangeTranslationData struct {
	TranslationID string `json:"translation_id"`
}

// ChatMessageInData is the inbound chat payload. Body is enforced ≤500 chars
// at the handler layer; over-cap → outbound MsgError with ErrCodeChatTooLong.
type ChatMessageInData struct {
	Body string `json:"body"`
}

// ChatReactionInData carries a single emoji from the curated whitelist
// (~24 anime-friendly emoji — list lives in handler, not here).
type ChatReactionInData struct {
	Emoji string `json:"emoji"`
}

// PresenceHeartbeatData is intentionally empty — the act of sending it bumps
// the sender's last_seen_at on the server side.
type PresenceHeartbeatData struct{}

// ----------------------------------------------------------------------------
// Outbound payload shapes (server → client).
// ----------------------------------------------------------------------------

// RoomStateChangedData mirrors the per-mutation broadcast after any
// state:change_* inbound. `Field` is the JSON tag from Room (e.g. "player",
// "translation_id", "episode_id"); Value is the new value (any JSON-ish type).
type RoomStateChangedData struct {
	Field    string      `json:"field"`
	Value    interface{} `json:"value"`
	ByUserID string      `json:"by_user_id"`
}

// PlaybackEventData is the broadcast shape for play / pause / seek inbounds.
// Kind ∈ {"play", "pause", "seek"}. ServerTS is the authoritative wall-clock
// (unix ms) at which the server processed the event — clients use it as the
// drift-detection anchor.
type PlaybackEventData struct {
	Kind     string  `json:"kind"`
	Time     float64 `json:"time"`
	ByUserID string  `json:"by_user_id"`
	ServerTS int64   `json:"server_ts"`
}

// PlaybackCorrectionData is the per-recipient drift-correction nudge.
// Soft (1.5s < drift ≤ 5s) and hard (drift > 5s) corrections share the same
// shape — the client decides how aggressively to apply it (nudge vs
// hard-seek). Persistent drift across 5 ticks → MsgError with
// ErrCodePersistentDrift and the server stops correcting (anti-spam).
type PlaybackCorrectionData struct {
	Time     float64 `json:"time"`
	ServerTS int64   `json:"server_ts"`
}

// MemberJoinedData announces a new member to the room. The full Member
// struct (including MemberMeta) is sent so the receiver can update its
// roster without a separate fetch.
type MemberJoinedData struct {
	UserID string     `json:"user_id"`
	Member MemberMeta `json:"member"`
}

// MemberLeftData announces a member's connection drop. Just the user_id —
// receivers already have the meta in their local roster from join.
type MemberLeftData struct {
	UserID string `json:"user_id"`
}

// ChatMessageOutData wraps a server-attributed ChatMessage for broadcast.
// Differs from ChatMessageInData by carrying the full message (with
// server-generated ID, server-stamped TS, and sender user_id/username).
type ChatMessageOutData struct {
	Message ChatMessage `json:"message"`
}

// ChatReactionOutData broadcasts a single reaction burst. Not persisted —
// purely for UI overlay (the 3s fade-out anim is client-side).
type ChatReactionOutData struct {
	UserID string `json:"user_id"`
	Emoji  string `json:"emoji"`
}

// RoomClosedData is the final outbound before the WS is closed. Reason is a
// short machine-readable token (e.g. "host_closed", "grace_period_expired").
type RoomClosedData struct {
	Reason string `json:"reason"`
}

// ErrorData is the universal outbound error envelope payload. Code is one
// of the ErrCode* constants above; Message is human-readable (optional);
// Hint suggests recovery action (e.g. "reload" on PERSISTENT_DRIFT).
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}
