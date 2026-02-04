package domain

import "time"

type WatchProgress struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        string    `gorm:"type:uuid;index" json:"user_id"`
	AnimeID       string    `gorm:"type:uuid;index" json:"anime_id"`
	EpisodeNumber int       `gorm:"index" json:"episode_number"`
	Progress      int       `json:"progress"`
	Duration      int       `json:"duration"`
	Completed     bool      `gorm:"default:false" json:"completed"`
	LastWatchedAt time.Time `json:"last_watched_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (WatchProgress) TableName() string {
	return "watch_progress"
}

type AnimeListEntry struct {
	ID                 string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID             string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime" json:"user_id"`
	AnimeID            string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime" json:"anime_id"`
	AnimeTitle         string     `gorm:"size:500" json:"anime_title"`
	AnimeCover         string     `gorm:"type:text" json:"anime_cover"`
	Status             string     `gorm:"size:20;default:'watching';index" json:"status"`
	Score              int        `json:"score"`
	Episodes           int        `json:"episodes"`
	Notes              string     `gorm:"type:text" json:"notes"`
	Tags               string     `json:"tags"`
	IsRewatching       bool       `gorm:"default:false" json:"is_rewatching"`
	Priority           string     `gorm:"size:20" json:"priority"`
	AnimeType          string     `gorm:"size:20" json:"anime_type"`
	AnimeTotalEpisodes int        `json:"anime_total_episodes"`
	MalID              *int       `json:"mal_id,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (AnimeListEntry) TableName() string {
	return "anime_list"
}

type WatchHistory struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        string    `gorm:"type:uuid;index" json:"user_id"`
	AnimeID       string    `gorm:"type:uuid;index" json:"anime_id"`
	EpisodeNumber int       `json:"episode_number"`
	WatchedAt     time.Time `json:"watched_at"`
}

func (WatchHistory) TableName() string {
	return "watch_history"
}

type Review struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     string    `gorm:"type:uuid;index;uniqueIndex:idx_user_anime_review" json:"user_id"`
	AnimeID    string    `gorm:"type:uuid;index;uniqueIndex:idx_user_anime_review" json:"anime_id"`
	AnimeTitle string    `gorm:"size:500" json:"anime_title"`
	AnimeCover string    `gorm:"type:text" json:"anime_cover"`
	Username   string    `gorm:"size:32" json:"username"`
	Score      int       `json:"score"`
	ReviewText string    `gorm:"type:text" json:"review_text"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Review) TableName() string {
	return "reviews"
}

// Request/Response types (not database tables)
type UpdateProgressRequest struct {
	AnimeID       string `json:"anime_id"`
	EpisodeNumber int    `json:"episode_number"`
	Progress      int    `json:"progress"`
	Duration      int    `json:"duration"`
}

type UpdateListRequest struct {
	AnimeID            string     `json:"anime_id"`
	AnimeTitle         string     `json:"anime_title,omitempty"`
	AnimeCover         string     `json:"anime_cover,omitempty"`
	Status             string     `json:"status"`
	Score              *int       `json:"score,omitempty"`
	Episodes           *int       `json:"episodes,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
	Tags               *string    `json:"tags,omitempty"`
	IsRewatching       *bool      `json:"is_rewatching,omitempty"`
	Priority           *string    `json:"priority,omitempty"`
	AnimeType          string     `json:"anime_type,omitempty"`
	AnimeTotalEpisodes *int       `json:"anime_total_episodes,omitempty"`
	MalID              *int       `json:"mal_id,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type AnimeRating struct {
	AnimeID      string  `json:"anime_id"`
	AverageScore float64 `json:"average_score"`
	TotalReviews int     `json:"total_reviews"`
}

type CreateReviewRequest struct {
	AnimeID    string `json:"anime_id"`
	AnimeTitle string `json:"anime_title,omitempty"`
	AnimeCover string `json:"anime_cover,omitempty"`
	Score      int    `json:"score"`
	ReviewText string `json:"review_text"`
}
