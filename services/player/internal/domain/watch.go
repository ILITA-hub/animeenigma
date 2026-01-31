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
	ID                 string     `json:"id" db:"id"`
	UserID             string     `json:"user_id" db:"user_id"`
	AnimeID            string     `json:"anime_id" db:"anime_id"`
	AnimeTitle         string     `json:"anime_title" db:"anime_title"`
	AnimeCover         string     `json:"anime_cover" db:"anime_cover"`
	Status             string     `json:"status" db:"status"` // watching, completed, plan_to_watch, dropped, on_hold
	Score              int        `json:"score" db:"score"`   // 1-10
	Episodes           int        `json:"episodes" db:"episodes"`
	Notes              string     `json:"notes" db:"notes"`
	Tags               string     `json:"tags" db:"tags"`
	IsRewatching       bool       `json:"is_rewatching" db:"is_rewatching"`
	Priority           string     `json:"priority" db:"priority"` // low, medium, high
	AnimeType          string     `json:"anime_type" db:"anime_type"`
	AnimeTotalEpisodes int        `json:"anime_total_episodes" db:"anime_total_episodes"`
	MalID              *int       `json:"mal_id,omitempty" db:"mal_id"`
	StartedAt          *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
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
	AnimeID            string  `json:"anime_id"`
	AnimeTitle         string  `json:"anime_title,omitempty"`
	AnimeCover         string  `json:"anime_cover,omitempty"`
	Status             string  `json:"status"`
	Score              *int    `json:"score,omitempty"`
	Episodes           *int    `json:"episodes,omitempty"`
	Notes              *string `json:"notes,omitempty"`
	Tags               *string `json:"tags,omitempty"`
	IsRewatching       *bool   `json:"is_rewatching,omitempty"`
	Priority           *string `json:"priority,omitempty"`
	AnimeType          string  `json:"anime_type,omitempty"`
	AnimeTotalEpisodes *int    `json:"anime_total_episodes,omitempty"`
	MalID              *int    `json:"mal_id,omitempty"`
}

// Review represents a user review for an anime
type Review struct {
	ID         string    `json:"id" db:"id"`
	UserID     string    `json:"user_id" db:"user_id"`
	AnimeID    string    `json:"anime_id" db:"anime_id"`
	AnimeTitle string    `json:"anime_title" db:"anime_title"`
	AnimeCover string    `json:"anime_cover" db:"anime_cover"`
	Username   string    `json:"username" db:"username"`
	Score      int       `json:"score" db:"score"` // 1-10
	ReviewText string    `json:"review_text" db:"review_text"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// AnimeRating contains aggregated rating info
type AnimeRating struct {
	AnimeID      string  `json:"anime_id" db:"anime_id"`
	AverageScore float64 `json:"average_score" db:"average_score"`
	TotalReviews int     `json:"total_reviews" db:"total_reviews"`
}

// CreateReviewRequest for creating/updating reviews
type CreateReviewRequest struct {
	AnimeID    string `json:"anime_id"`
	AnimeTitle string `json:"anime_title,omitempty"`
	AnimeCover string `json:"anime_cover,omitempty"`
	Score      int    `json:"score"`
	ReviewText string `json:"review_text"`
}
