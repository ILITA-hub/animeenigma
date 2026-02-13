package domain

import (
	"time"

	"gorm.io/gorm"
)

// Anime represents an anime in the catalog
type Anime struct {
	ID              string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name            string         `gorm:"size:500;index" json:"name"`
	NameRU          string         `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP          string         `gorm:"size:500" json:"name_jp,omitempty"`
	Description     string         `gorm:"type:text" json:"description,omitempty"`
	Year            int            `json:"year,omitempty"`
	Season          string         `gorm:"size:20" json:"season,omitempty"`
	Status          AnimeStatus    `gorm:"size:20;default:'released'" json:"status"`
	EpisodesCount   int            `json:"episodes_count"`
	EpisodesAired   int            `json:"episodes_aired,omitempty"`
	EpisodeDuration int            `json:"episode_duration,omitempty"`
	Score           float64        `gorm:"type:decimal(4,2)" json:"score,omitempty"`
	PosterURL       string         `gorm:"type:text" json:"poster_url,omitempty"`
	ShikimoriID     string         `gorm:"size:50;index" json:"shikimori_id,omitempty"`
	MALID           string         `gorm:"size:50" json:"mal_id,omitempty"`
	AniListID       string         `gorm:"size:50" json:"anilist_id,omitempty"`
	HasVideo        bool           `gorm:"default:false;index" json:"has_video"`
	Hidden          bool           `gorm:"default:false" json:"hidden"`
	NextEpisodeAt   *time.Time     `json:"next_episode_at,omitempty"`
	AiredOn         *time.Time     `json:"aired_on,omitempty"`
	Genres          []Genre        `gorm:"many2many:anime_genres;" json:"genres,omitempty"`
	Videos          []Video        `gorm:"foreignKey:AnimeID" json:"-"`
	VideoSources    []VideoSource  `gorm:"-" json:"video_sources,omitempty"` // Computed, not stored
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
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
	ID        string    `gorm:"size:50;primaryKey" json:"id"` // Shikimori genre ID
	Name      string    `gorm:"size:100;uniqueIndex" json:"name"`
	NameRU    string    `gorm:"size:100" json:"name_ru,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Episode represents an anime episode
type Episode struct {
	ID        string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID   string     `gorm:"type:uuid;index" json:"anime_id"`
	Number    int        `json:"number"`
	Name      string     `gorm:"size:500" json:"name,omitempty"`
	NameJP    string     `gorm:"size:500" json:"name_jp,omitempty"`
	AiredAt   *time.Time `json:"aired_at,omitempty"`
	Duration  int        `json:"duration,omitempty"`
	HasVideo  bool       `json:"has_video"`
	CreatedAt time.Time  `json:"created_at"`
}

// Video represents a video file (episode, opening, or ending)
type Video struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID       string     `gorm:"type:uuid;index" json:"anime_id"`
	Type          VideoType  `gorm:"size:20" json:"type"`
	EpisodeNumber int        `json:"episode_number,omitempty"`
	Name          string     `gorm:"size:500" json:"name,omitempty"`
	SourceType    SourceType `gorm:"size:20" json:"source_type"`
	SourceURL     string     `gorm:"type:text" json:"source_url,omitempty"`
	StorageKey    string     `gorm:"size:500" json:"storage_key,omitempty"`
	Quality       string     `gorm:"size:20" json:"quality,omitempty"`
	Language      string     `gorm:"size:50;default:'japanese'" json:"language,omitempty"`
	Duration      int        `json:"duration,omitempty"`
	ThumbnailURL  string     `gorm:"type:text" json:"thumbnail_url,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// PinnedTranslation represents a pinned translation for an anime
type PinnedTranslation struct {
	AnimeID          string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	TranslationID    int       `gorm:"primaryKey" json:"translation_id"`
	TranslationTitle string    `gorm:"size:255" json:"translation_title"`
	TranslationType  string    `gorm:"size:50" json:"translation_type"`
	PinnedAt         time.Time `json:"pinned_at"`
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
	Type          string `json:"type"`
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
	Type          string `json:"type"`
	EpisodesCount int    `json:"episodes_count"`
	Pinned        bool   `json:"pinned"`
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
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Link          string            `json:"link"`
	Title         string            `json:"title"`
	TitleOrig     string            `json:"title_orig"`
	Year          int               `json:"year"`
	EpisodesCount int               `json:"episodes_count,omitempty"`
	ShikimoriID   string            `json:"shikimori_id,omitempty"`
	Translation   *KodikTranslation `json:"translation"`
	Quality       string            `json:"quality"`
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
	Source   string
}

// CreateAnimeRequest for admin anime creation
type CreateAnimeRequest struct {
	Name          string   `json:"name" validate:"required"`
	NameRU        string   `json:"name_ru"`
	NameJP        string   `json:"name_jp"`
	Description   string   `json:"description"`
	Year          int      `json:"year"`
	Season        string   `json:"season"`
	Status        string   `json:"status"`
	EpisodesCount int      `json:"episodes_count"`
	PosterURL     string   `json:"poster_url"`
	GenreIDs      []string `json:"genre_ids"`
	ShikimoriID   string   `json:"shikimori_id"`
	MALID         string   `json:"mal_id"`
}

// AddVideoRequest for adding video sources
type AddVideoRequest struct {
	EpisodeNumber int        `json:"episode_number" validate:"required"`
	SourceType    SourceType `json:"source_type" validate:"required"`
	ExternalURL   string     `json:"external_url"`
	Quality       string     `json:"quality"`
	Language      string     `json:"language"`
	Subtitles     []string   `json:"subtitles"`
}

// HiAnimeEpisode represents an episode from HiAnime
type HiAnimeEpisode struct {
	ID       string `json:"id"`       // e.g., "death-note-60?ep=1234"
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"is_filler"`
}

// HiAnimeServer represents a streaming server from HiAnime
type HiAnimeServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "sub", "dub", "raw"
}

// HiAnimeSubtitle represents a subtitle track
type HiAnimeSubtitle struct {
	URL     string `json:"url"`
	Lang    string `json:"lang"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// HiAnimeTimeRange for intro/outro markers
type HiAnimeTimeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// HiAnimeStream represents stream source data from HiAnime
type HiAnimeStream struct {
	URL       string              `json:"url"`
	Type      string              `json:"type"` // "hls", "mp4", or "iframe"
	Subtitles []HiAnimeSubtitle   `json:"subtitles,omitempty"`
	Headers   map[string]string   `json:"headers,omitempty"`
	Intro     *HiAnimeTimeRange   `json:"intro,omitempty"`
	Outro     *HiAnimeTimeRange   `json:"outro,omitempty"`
}

// HiAnimeSearchResult represents a search result from HiAnime
type HiAnimeSearchResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Poster   string `json:"poster"`
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// ConsumetEpisode represents an episode from Consumet
type ConsumetEpisode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"is_filler"`
}

// ConsumetServer represents a streaming server from Consumet
type ConsumetServer struct {
	Name string `json:"name"`
}

// ConsumetSubtitle represents a subtitle track
type ConsumetSubtitle struct {
	URL  string `json:"url"`
	Lang string `json:"lang"`
}

// ConsumetStream represents stream source data from Consumet
type ConsumetStream struct {
	URL       string             `json:"url"`
	IsM3U8    bool               `json:"isM3U8"`
	Quality   string             `json:"quality"`
	Headers   map[string]string  `json:"headers,omitempty"`
	Subtitles []ConsumetSubtitle `json:"subtitles,omitempty"`
}

// ConsumetSearchResult represents a search result from Consumet
type ConsumetSearchResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Image    string `json:"image"`
	Type     string `json:"type"`
	SubOrDub string `json:"subOrDub"`
}

// JimakuSubtitle represents a Japanese subtitle file from Jimaku
type JimakuSubtitle struct {
	URL      string `json:"url"`       // Direct download URL from jimaku.cc
	FileName string `json:"file_name"` // Original filename (e.g. "EP01.ass")
	Lang     string `json:"lang"`      // Always "Japanese"
	Format   string `json:"format"`    // "ass", "srt", etc.
}

// JimakuSubtitleResponse represents the response for Jimaku subtitle lookup
type JimakuSubtitleResponse struct {
	Subtitles []JimakuSubtitle `json:"subtitles"`
	EntryName string           `json:"entry_name,omitempty"`
}

// MALResolveResult represents the result of resolving a MAL ID
type MALResolveResult struct {
	Status   string `json:"status"`              // "resolved" or "ambiguous"
	Anime    *Anime `json:"anime,omitempty"`     // set when resolved
	MALTitle string `json:"mal_title,omitempty"` // set when ambiguous
	MALID    string `json:"mal_id,omitempty"`    // always set
}
