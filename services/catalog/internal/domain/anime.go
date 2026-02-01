package domain

import (
	"time"
)

// Anime represents an anime in the catalog
type Anime struct {
	ID              string        `db:"id" json:"id"`
	Name            string        `db:"name" json:"name"`
	NameRU          string        `db:"name_ru" json:"name_ru,omitempty"`
	NameJP          string        `db:"name_jp" json:"name_jp,omitempty"`
	Description     string        `db:"description" json:"description,omitempty"`
	Year            int           `db:"year" json:"year,omitempty"`
	Season          string        `db:"season" json:"season,omitempty"`
	Status          AnimeStatus   `db:"status" json:"status"`
	EpisodesCount   int           `db:"episodes_count" json:"episodes_count"`
	EpisodesAired   int           `db:"episodes_aired" json:"episodes_aired,omitempty"`
	EpisodeDuration int           `db:"episode_duration" json:"episode_duration,omitempty"`
	Score           float64       `db:"score" json:"score,omitempty"`
	PosterURL       string        `db:"poster_url" json:"poster_url,omitempty"`
	ShikimoriID     string        `db:"shikimori_id" json:"shikimori_id,omitempty"`
	MALID           string        `db:"mal_id" json:"mal_id,omitempty"`
	AniListID       string        `db:"anilist_id" json:"anilist_id,omitempty"`
	HasVideo        bool          `db:"has_video" json:"has_video"`
	Hidden          bool          `db:"hidden" json:"hidden"`
	NextEpisodeAt   *time.Time    `db:"next_episode_at" json:"next_episode_at,omitempty"`
	AiredOn         *time.Time    `db:"aired_on" json:"aired_on,omitempty"`
	Genres          []Genre       `json:"genres,omitempty"`
	VideoSources    []VideoSource `json:"video_sources,omitempty"`
	CreatedAt       time.Time     `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time     `db:"updated_at" json:"updated_at"`
}

// AnimeStatus represents the airing status
type AnimeStatus string

const (
	StatusOngoing   AnimeStatus = "ongoing"
	StatusReleased  AnimeStatus = "released"
	StatusAnnounced AnimeStatus = "announced"
)

// Genre represents an anime genre
type Genre struct {
	ID     string `db:"id" json:"id"`
	Name   string `db:"name" json:"name"`
	NameRU string `db:"name_ru" json:"name_ru,omitempty"`
}

// Episode represents an anime episode
type Episode struct {
	ID        string    `db:"id" json:"id"`
	AnimeID   string    `db:"anime_id" json:"anime_id"`
	Number    int       `db:"number" json:"number"`
	Name      string    `db:"name" json:"name,omitempty"`
	NameJP    string    `db:"name_jp" json:"name_jp,omitempty"`
	AiredAt   *time.Time `db:"aired_at" json:"aired_at,omitempty"`
	Duration  int       `db:"duration" json:"duration,omitempty"`
	HasVideo  bool      `db:"has_video" json:"has_video"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Video represents a video file (episode, opening, or ending)
type Video struct {
	ID            string      `db:"id" json:"id"`
	AnimeID       string      `db:"anime_id" json:"anime_id"`
	AnimeName     string      `json:"anime_name,omitempty"` // Populated from join
	Type          VideoType   `db:"type" json:"type"`
	EpisodeNumber int         `db:"episode_number" json:"episode_number,omitempty"`
	Name          string      `db:"name" json:"name,omitempty"`
	SourceType    SourceType  `db:"source_type" json:"source_type"`
	SourceURL     string      `db:"source_url" json:"source_url,omitempty"`
	StorageKey    string      `db:"storage_key" json:"storage_key,omitempty"`
	Quality       string      `db:"quality" json:"quality,omitempty"`
	Language      string      `db:"language" json:"language,omitempty"`
	Duration      int         `db:"duration" json:"duration,omitempty"`
	ThumbnailURL  string      `db:"thumbnail_url" json:"thumbnail_url,omitempty"`
	CreatedAt     time.Time   `db:"created_at" json:"created_at"`
}

// VideoType represents the type of video
type VideoType string

const (
	VideoTypeEpisode VideoType = "episode"
	VideoTypeOpening VideoType = "opening"
	VideoTypeEnding  VideoType = "ending"
)

// SourceType represents where the video is stored/streamed from
type SourceType string

const (
	SourceTypeMinio    SourceType = "minio"
	SourceTypeExternal SourceType = "external"
	SourceTypeAniboom  SourceType = "aniboom"
	SourceTypeKodik    SourceType = "kodik"
)

// AniboomVideoSource represents a video source from Aniboom
type AniboomVideoSource struct {
	URL           string `json:"url"`
	Type          string `json:"type"` // "mpd" or "m3u8"
	Episode       int    `json:"episode"`
	Translation   string `json:"translation"`
	TranslationID string `json:"translation_id"`
}

// AniboomTranslation represents an available translation/dubbing
type AniboomTranslation struct {
	Name          string `json:"name"`
	TranslationID string `json:"translation_id"`
}

// KodikTranslation represents an available translation/dubbing from Kodik
type KodikTranslation struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Type          string `json:"type"`           // "voice" or "subtitles"
	EpisodesCount int    `json:"episodes_count"` // Number of available episodes for this translation
	Pinned        bool   `json:"pinned"`         // Whether this translation is pinned for this anime
}

// PinnedTranslation represents a pinned translation for an anime
type PinnedTranslation struct {
	AnimeID          string    `db:"anime_id" json:"anime_id"`
	TranslationID    int       `db:"translation_id" json:"translation_id"`
	TranslationTitle string    `db:"translation_title" json:"translation_title"`
	TranslationType  string    `db:"translation_type" json:"translation_type"`
	PinnedAt         time.Time `db:"pinned_at" json:"pinned_at"`
}

// PinTranslationRequest for pinning a translation
type PinTranslationRequest struct {
	TranslationID    int    `json:"translation_id" validate:"required"`
	TranslationTitle string `json:"translation_title"`
	TranslationType  string `json:"translation_type"`
}

// KodikVideoSource represents a video source from Kodik
type KodikVideoSource struct {
	EmbedLink     string `json:"embed_link"`
	Episode       int    `json:"episode"`
	TranslationID int    `json:"translation_id"`
	Translation   string `json:"translation"`
	Quality       string `json:"quality,omitempty"`
}

// KodikSearchResult represents a search result from Kodik
type KodikSearchResult struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Link          string             `json:"link"`
	Title         string             `json:"title"`
	TitleOrig     string             `json:"title_orig"`
	Year          int                `json:"year"`
	EpisodesCount int                `json:"episodes_count,omitempty"`
	ShikimoriID   string             `json:"shikimori_id,omitempty"`
	Translation   *KodikTranslation  `json:"translation"`
	Quality       string             `json:"quality"`
}

// VideoSource is a summary of available video sources for an episode
type VideoSource struct {
	Type      SourceType `json:"type"`
	Quality   string     `json:"quality"`
	Language  string     `json:"language"`
	Subtitles []string   `json:"subtitles,omitempty"`
}

// ExternalIDs holds external database IDs
type ExternalIDs struct {
	Shikimori string `json:"shikimori,omitempty"`
	MAL       string `json:"mal,omitempty"`
	AniList   string `json:"anilist,omitempty"`
	AniDB     string `json:"anidb,omitempty"`
}

// SearchFilters for anime search
type SearchFilters struct {
	Query    string
	Year     *int
	YearFrom *int
	YearTo   *int
	Season   string
	Status   AnimeStatus
	GenreIDs []string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// CreateAnimeRequest for admin anime creation
type CreateAnimeRequest struct {
	Name        string   `json:"name" validate:"required"`
	NameRU      string   `json:"name_ru"`
	NameJP      string   `json:"name_jp"`
	Description string   `json:"description"`
	Year        int      `json:"year"`
	Season      string   `json:"season"`
	Status      string   `json:"status"`
	EpisodesCount int    `json:"episodes_count"`
	PosterURL   string   `json:"poster_url"`
	GenreIDs    []string `json:"genre_ids"`
	ShikimoriID string   `json:"shikimori_id"` // If provided, fetch from Shikimori
}

// AddVideoRequest for adding video sources
type AddVideoRequest struct {
	EpisodeNumber int        `json:"episode_number" validate:"required"`
	SourceType    SourceType `json:"source_type" validate:"required"`
	ExternalURL   string     `json:"external_url"` // Required if source_type is external
	Quality       string     `json:"quality"`
	Language      string     `json:"language"`
	Subtitles     []string   `json:"subtitles"`
}
