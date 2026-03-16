package domain

import "time"

// GenreInfo is a read-only projection of the genres table.
type GenreInfo struct {
	ID     string `gorm:"size:50;primaryKey" json:"id"`
	Name   string `json:"name"`
	NameRU string `json:"name_ru,omitempty"`
}

func (GenreInfo) TableName() string { return "genres" }

// AnimeInfo is a read-only projection of the animes table.
// It omits DeletedAt so GORM won't add "WHERE deleted_at IS NULL",
// ensuring entries referencing soft-deleted anime still return data.
type AnimeInfo struct {
	ID            string      `gorm:"type:uuid;primaryKey" json:"id"`
	Name          string      `json:"name"`
	NameRU        string      `json:"name_ru,omitempty"`
	NameJP        string      `json:"name_jp,omitempty"`
	PosterURL     string      `json:"poster_url,omitempty"`
	EpisodesCount int         `json:"episodes_count"`
	EpisodesAired int         `json:"episodes_aired,omitempty"`
	Genres        []GenreInfo `gorm:"many2many:anime_genres;joinForeignKey:anime_id;joinReferences:genre_id" json:"genres,omitempty"`
}

func (AnimeInfo) TableName() string { return "animes" }

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
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime" json:"user_id"`
	AnimeID      string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime" json:"anime_id"`
	Anime        *AnimeInfo `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
	Status       string     `gorm:"size:20;default:'watching';index" json:"status"`
	Score        int        `json:"score"`
	Episodes     int        `json:"episodes"`
	Notes        string     `gorm:"type:text" json:"notes"`
	Tags         string     `json:"tags"`
	IsRewatching bool       `gorm:"default:false" json:"is_rewatching"`
	Priority     string     `gorm:"size:20" json:"priority"`
	MalID        *int       `json:"mal_id,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
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
	ID         string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime_review" json:"user_id"`
	AnimeID    string     `gorm:"type:uuid;index;uniqueIndex:idx_user_anime_review" json:"anime_id"`
	Anime      *AnimeInfo `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
	Username   string     `gorm:"size:32" json:"username"`
	Score      int        `json:"score"`
	ReviewText string     `gorm:"type:text" json:"review_text"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
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
	AnimeID      string     `json:"anime_id"`
	Status       string     `json:"status"`
	Score        *int       `json:"score,omitempty"`
	Episodes     *int       `json:"episodes,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	Tags         *string    `json:"tags,omitempty"`
	IsRewatching *bool      `json:"is_rewatching,omitempty"`
	Priority     *string    `json:"priority,omitempty"`
	MalID        *int       `json:"mal_id,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type AnimeRating struct {
	AnimeID      string  `json:"anime_id"`
	AverageScore float64 `json:"average_score"`
	TotalReviews int     `json:"total_reviews"`
}

type CreateReviewRequest struct {
	AnimeID    string `json:"anime_id"`
	Score      int    `json:"score"`
	ReviewText string `json:"review_text"`
}

// PaginationParams holds pagination and sorting options
type PaginationParams struct {
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	Sort    string `json:"sort"`
	Order   string `json:"order"`
}

// Validate checks pagination params and applies defaults
func (p *PaginationParams) Validate() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 || p.PerPage > 100 {
		p.PerPage = 24
	}
	allowedSorts := map[string]bool{
		"updated_at": true,
		"created_at": true,
		"score":      true,
		"status":     true,
	}
	if !allowedSorts[p.Sort] {
		p.Sort = "updated_at"
	}
	if p.Order != "asc" {
		p.Order = "desc"
	}
}

// Offset returns the SQL offset
func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

// AnimeStatusEntry is a lightweight entry for the status map
type AnimeStatusEntry struct {
	AnimeID string `json:"anime_id" gorm:"column:anime_id"`
	Status  string `json:"status" gorm:"column:status"`
}
