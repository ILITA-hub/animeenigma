package domain

import (
	"time"

	"gorm.io/gorm"
)

type AnimeTheme struct {
	ID              string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ExternalID      int            `gorm:"uniqueIndex;not null" json:"external_id"`
	AnimeName       string         `gorm:"type:text;not null" json:"anime_name"`
	AnimeSlug       string         `gorm:"type:text" json:"anime_slug"`
	PosterURL       string         `gorm:"type:text" json:"poster_url,omitempty"`
	ThemeType       string         `gorm:"type:text;not null" json:"theme_type"` // "OP" or "ED"
	Sequence        int            `json:"sequence"`
	Slug            string         `gorm:"type:text" json:"slug"` // "OP1", "ED2"
	SongTitle       string         `gorm:"type:text" json:"song_title,omitempty"`
	ArtistName      string         `gorm:"type:text" json:"artist_name,omitempty"`
	VideoBasename   string         `gorm:"type:text" json:"video_basename,omitempty"`
	VideoResolution int            `json:"video_resolution,omitempty"`
	AudioBasename   string         `gorm:"type:text" json:"audio_basename,omitempty"`
	MALID           int            `gorm:"index" json:"mal_id,omitempty"`
	Year            int            `gorm:"not null;index" json:"year"`
	Season          string         `gorm:"type:text;not null;index" json:"season"` // "winter","spring","summer","fall"
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields (not stored in DB, but scanned from queries)
	AvgScore  float64 `gorm:"->;-:migration" json:"avg_score"`
	VoteCount int     `gorm:"->;-:migration" json:"vote_count"`
	UserScore *int    `gorm:"->;-:migration" json:"user_score,omitempty"`
	AnimeID   string  `gorm:"->;-:migration" json:"anime_id,omitempty"` // Local catalog anime UUID (joined from animes table)
}

func (AnimeTheme) TableName() string {
	return "anime_themes"
}

type ThemeRating struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_theme" json:"user_id"`
	ThemeID   string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_theme" json:"theme_id"`
	Score     int       `gorm:"not null" json:"score"` // 1-10
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ThemeRating) TableName() string {
	return "theme_ratings"
}

type RateRequest struct {
	Score int `json:"score"`
}

type ThemeListParams struct {
	Year   int    `json:"year"`
	Season string `json:"season"`
	Type   string `json:"type"` // "op", "ed", or ""
	Sort   string `json:"sort"` // "rating", "name", "newest"
	UserID string `json:"-"`
}
