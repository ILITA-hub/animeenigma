// Package domain holds the canonical state shapes for the watch-together
// service. These types mirror the Redis schema in
// .planning/workstreams/watch-together/phases/01-backend-foundation/01-CONTEXT.md
// §Redis State Schema and the protocol in
// docs/superpowers/specs/2026-05-25-watch-together-design.md §WebSocket Protocol.
//
// No methods — pure data carriers. Validation lives in handler/service layers
// (project convention; see services/notifications/internal/domain/).
package domain

// Room is the canonical Redis HASH `wt:room:{roomId}`. Persisted with a sliding
// 900s TTL; expires naturally once the last member disconnects and the 5min
// grace period elapses (01-CONTEXT.md §Redis State Schema).
type Room struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"` // unix seconds

	AnimeID       string `json:"anime_id"`
	EpisodeID     string `json:"episode_id"`
	Player        string `json:"player"` // PlayerKodik | PlayerAnimeLib | PlayerOurEnglish | PlayerHanime | PlayerAePlayer
	TranslationID string `json:"translation_id"`

	PlaybackState           string  `json:"playback_state"`            // StatePlaying | StatePaused
	PlaybackTime            float64 `json:"playback_time"`             // seconds into the episode
	PlaybackTimeUpdatedAtMs int64   `json:"playback_time_updated_at"`  // unix milliseconds — drift-detection anchor
	HostUserID              string  `json:"host_user_id"`              // cosmetic only — any member can drive playback
}

// MemberMeta is the value half of the `wt:room:{roomId}:members` HASH;
// keys are user IDs and values are this struct JSON-encoded.
type MemberMeta struct {
	Username   string `json:"username"`
	AvatarURL  string `json:"avatar_url"`
	JoinedAt   int64  `json:"joined_at"`    // unix sec
	LastSeenAt int64  `json:"last_seen_at"` // unix sec — bumped on every presence:heartbeat
}

// Member is the on-the-wire pairing of user_id + their MemberMeta. Used in
// RoomSnapshot.Members and member:joined / member:left envelope payloads.
type Member struct {
	UserID string     `json:"user_id"`
	Meta   MemberMeta `json:"meta"`
}

// RoomSnapshot is the payload of the outbound `room:snapshot` envelope sent
// to a freshly-connected client. Includes the canonical room state, full
// member roster, the last 50 chat messages (capped by retrieval, not by
// storage — Redis keeps 100 per the LTRIM cap), and a protocol version so
// clients can reject incompatible servers.
type RoomSnapshot struct {
	Room            Room          `json:"room"`
	Members         []Member      `json:"members"`
	Messages        []ChatMessage `json:"messages"`
	ProtocolVersion string        `json:"protocol_version"` // always ProtocolVersion const
}
