package domain

import "time"

type WatchProgress struct {
	ID               string    `json:"id" db:"id"`
	UserID           string    `json:"user_id" db:"user_id"`
	AnimeID          string    `json:"anime_id" db:"anime_id"`
	EpisodeNumber    int       `json:"episode_number" db:"episode_number"`
	Progress         int       `json:"progress" db:"progress"`          // seconds
	Duration         int       `json:"duration" db:"duration"`          // seconds
	Completed        bool      `json:"completed" db:"completed"`
	LastWatchedAt    time.Time `json:"last_watched_at" db:"last_watched_at"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type AnimeListEntry struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	AnimeID   string    `json:"anime_id" db:"anime_id"`
	Status    string    `json:"status" db:"status"` // watching, completed, plan_to_watch, dropped, on_hold
	Score     int       `json:"score" db:"score"`   // 1-10
	Episodes  int       `json:"episodes" db:"episodes"`
	Notes     string    `json:"notes" db:"notes"`
	StartedAt *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type WatchHistory struct {
	ID            string    `json:"id" db:"id"`
	UserID        string    `json:"user_id" db:"user_id"`
	AnimeID       string    `json:"anime_id" db:"anime_id"`
	EpisodeNumber int       `json:"episode_number" db:"episode_number"`
	WatchedAt     time.Time `json:"watched_at" db:"watched_at"`
}

// Request/Response types
type UpdateProgressRequest struct {
	AnimeID       string `json:"anime_id"`
	EpisodeNumber int    `json:"episode_number"`
	Progress      int    `json:"progress"`
	Duration      int    `json:"duration"`
}

type UpdateListRequest struct {
	AnimeID  string  `json:"anime_id"`
	Status   string  `json:"status"`
	Score    *int    `json:"score,omitempty"`
	Episodes *int    `json:"episodes,omitempty"`
	Notes    *string `json:"notes,omitempty"`
}
