package domain

// ChatMessage is a single chat entry. Persisted in the
// `wt:room:{roomId}:messages` LIST (LPUSH + LTRIM 0 99 — cap 100; the snapshot
// returns the most recent 50 to new joiners).
//
// Body is capped at 500 chars at the handler layer (over-cap → outbound
// error envelope with code ErrCodeChatTooLong, sender-only).
type ChatMessage struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Body     string `json:"body"`
	TS       int64  `json:"ts"` // unix milliseconds
}
