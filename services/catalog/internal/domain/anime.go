package domain

import (
	"time"

	"gorm.io/gorm"
)

// Anime represents an anime in the catalog
type Anime struct {
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"size:500;index" json:"name"`
	NameEN      string `gorm:"size:500" json:"name_en,omitempty"`
	NameRU      string `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP      string `gorm:"size:500" json:"name_jp,omitempty"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	// Year / Season / Status are filtered (and Status often ordered) in
	// Search / GetOngoingAnime / the next-episode + stale-refresh queries.
	// Previously unindexed → seq scans on every browse. (audit L389)
	Year   int         `gorm:"index" json:"year,omitempty"`
	Season string      `gorm:"size:20;index" json:"season,omitempty"`
	Status AnimeStatus `gorm:"size:20;default:'released';index" json:"status"`
	// S5 attribute dimensions (Phase 12 Decision §A1) — Shikimori-sourced.
	// Rating doubles as the S5 "demographic" proxy per Decision §A3.
	// MaterialSource column name avoids collision with VideoSource.SourceType.
	Kind           string `gorm:"size:20;index" json:"kind,omitempty"`
	Rating         string `gorm:"size:20" json:"rating,omitempty"`
	MaterialSource string `gorm:"size:50;column:material_source" json:"material_source,omitempty"`
	Franchise      string `gorm:"size:200;index" json:"franchise,omitempty"`
	// FranchiseChecked records that franchise backfill ran for this row, so a
	// genuinely standalone anime (empty franchise) is not re-fetched on every
	// guess-pool build. Internal bookkeeping — not exposed in the API.
	FranchiseChecked bool `gorm:"default:false;index" json:"-"`
	EpisodesCount    int  `json:"episodes_count"`
	EpisodesAired    int  `json:"episodes_aired,omitempty"`
	EpisodeDuration  int  `json:"episode_duration,omitempty"`
	// Composite index on (sort_priority, score) backs the default Search order
	// `sort_priority DESC, score DESC` (audit L389). priority:1/2 sets column
	// order within the index; the index is ascending but Postgres can scan it
	// backwards for the DESC ordering.
	Score       float64 `gorm:"type:decimal(4,2);index:idx_animes_sort_score,priority:2" json:"score,omitempty"`
	PosterURL   string  `gorm:"type:text" json:"poster_url,omitempty"`
	ShikimoriID string  `gorm:"size:50;index" json:"shikimori_id,omitempty"`
	MALID       string  `gorm:"size:50" json:"mal_id,omitempty"`
	AniListID   string  `gorm:"size:50" json:"anilist_id,omitempty"`
	// IMDbID / TMDBID — workstream raw-jp, Phase 02. Resolved lazily via
	// Kitsu mappings on the first OpenSubtitles query for this anime.
	// Nullable: not every title has either mapping.
	IMDbID   *string `gorm:"size:50;index" json:"imdb_id,omitempty"`
	TMDBID   *string `gorm:"size:50;index" json:"tmdb_id,omitempty"`
	HasVideo bool    `gorm:"default:false;index" json:"has_video"`
	// HasDub indicates the anime has at least one Kodik translation with
	// type=="voice" (a dubbed track, as opposed to subtitled-only). Populated
	// lazily by GetKodikTranslations whenever the catalog service touches
	// Kodik on behalf of this anime. Default false; existing rows remain
	// valid until search-driven re-ingest backfills them. Phase 9 (UX-18).
	HasDub bool `gorm:"default:false;index" json:"has_dub"`
	// Phase 15 (UX-31) — per-provider availability booleans. Each parser
	// (kodik / animelib) lazily sets its corresponding flag the first time
	// the catalog touches that provider for the anime. Mirrors HasDub.
	// Default false; existing rows backfill over time.
	HasKodik    bool `gorm:"default:false;index;column:has_kodik" json:"has_kodik"`
	HasAnimeLib bool `gorm:"default:false;index;column:has_animelib" json:"has_animelib"`
	// HasRaw — raw Japanese audio available via the AllAnime parser
	// (workstream raw-jp, Phase 01). Lazily backfilled when the catalog
	// service first resolves a raw stream for the anime.
	HasRaw bool `gorm:"default:false;index;column:has_raw" json:"has_raw"`
	// HasEnglish — at least one English source resolvable via the scraper
	// microservice (gogoanime, animepahe, allanime, animekai). Lazily
	// backfilled by the catalog's scraper-episode resolver whenever any
	// scraper provider returns >= 1 episode for the anime. Mirrors the
	// HasKodik / HasAnimeLib / HasRaw lazy-backfill pattern.
	// Phase 26 (SCRAPER-HEAL-25, CONTEXT.md D5).
	HasEnglish   bool `gorm:"default:false;index;column:has_english" json:"has_english"`
	Hidden       bool `gorm:"default:false" json:"hidden"`
	SortPriority int  `gorm:"default:0;index:idx_animes_sort_score,priority:1" json:"sort_priority,omitempty"`
	// NextEpisodeAt is filtered + ordered in the next-episode query;
	// AiredOn is filtered + ordered in GetOngoingAnime / ListGuessPoolCandidates.
	// Both previously unindexed. (audit L389)
	NextEpisodeAt *time.Time `gorm:"index" json:"next_episode_at,omitempty"`
	// NextEpisodeSource records which provider supplied NextEpisodeAt:
	// "shikimori" (calendar default) or "anilist" (corroborated override when
	// AniList's broadcaster schedule is strictly later). Used by the batch
	// refresh guard to avoid clobbering an AniList correction. Auto-migrated.
	NextEpisodeSource string     `gorm:"size:16;default:'shikimori';column:next_episode_source" json:"next_episode_source,omitempty"`
	AiredOn           *time.Time `gorm:"index" json:"aired_on,omitempty"`
	Genres            []Genre    `gorm:"many2many:anime_genres;" json:"genres,omitempty"`
	// Phase 12 Decision §A1/A2 — Studios absorbs the producers role; no
	// separate Producers field exists in v2.0 (Decision §A2 collapses
	// the spec-§3.1 producers 0.05 into studios 0.25).
	Studios []Studio `gorm:"many2many:anime_studios;" json:"studios,omitempty"`
	// Phase 12 Decision §A4 — AniList-sourced tags. Explicit AnimeTag
	// join model preserves Rank (0-100) for v2.1 rank-weighted TF-IDF
	// even though v1 S5 ignores rank.
	Tags         []Tag          `gorm:"many2many:anime_tags;joinForeignKey:AnimeID;joinReferences:TagID" json:"tags,omitempty"`
	Videos       []Video        `gorm:"foreignKey:AnimeID" json:"-"`
	VideoSources []VideoSource  `gorm:"-" json:"video_sources,omitempty"` // Computed, not stored
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// RelatedAnime represents a related anime entry fetched from Shikimori (not stored in DB)
type RelatedAnime struct {
	ShikimoriID string  `json:"shikimori_id"`
	LocalID     string  `json:"local_id,omitempty"`
	Name        string  `json:"name"`
	NameRU      string  `json:"name_ru"`
	RelationRU  string  `json:"relation_ru"`
	RelationEN  string  `json:"relation_en"`
	Score       float64 `json:"score"`
	Status      string  `json:"status"`
	PosterURL   string  `json:"poster_url"`
	Year        int     `json:"year,omitempty"`
	Episodes    int     `json:"episodes,omitempty"`
}

// SimilarAnime represents a similar anime entry fetched from Shikimori
// (not stored in DB). Phase 13 (REC-SIG-06) — sibling of RelatedAnime, but the
// Shikimori /similar endpoint returns a flat array of anime objects (no
// relation wrapper), so this type omits Relation* fields.
type SimilarAnime struct {
	ShikimoriID string  `json:"shikimori_id"`
	LocalID     string  `json:"local_id,omitempty"`
	Name        string  `json:"name"`
	NameRU      string  `json:"name_ru"`
	Score       float64 `json:"score"`
	Episodes    int     `json:"episodes"`
	Status      string  `json:"status"`
	PosterURL   string  `json:"poster_url"`
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

// Studio is an anime production company. Sourced from Shikimori's
// studios { id name } GraphQL payload (Phase 12 Decision §A1).
//
// Note: Shikimori does not separate "studios" and "producers" — Decision
// §A2 collapses the spec-§3.1 producers 0.05 weight into the studios 0.25
// weight, so this single Studio dimension represents both. Producers as a
// separate dimension is deferred to v3.0.
type Studio struct {
	ID        string    `gorm:"size:50;primaryKey" json:"id"` // Shikimori studio ID
	Name      string    `gorm:"size:200;uniqueIndex" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tag is an anime descriptor sourced from AniList (Phase 12 Decision §A4).
// Tag.ID is the slugified name (deterministic, idempotent — see
// services/catalog/internal/parser/anilist/client.go::SlugifyTagName).
// Source defaults to 'anilist' but is left open for future MAL/Shikimori
// keyword sources without schema change.
type Tag struct {
	ID        string    `gorm:"size:200;primaryKey" json:"id"`
	Name      string    `gorm:"size:200;index" json:"name"`
	Source    string    `gorm:"size:50;default:'anilist';index" json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnimeTag is the explicit join row for the anime <-> tag many-to-many.
// Rank preserves AniList's per-anime tag rank (0-100). Decision §A4
// ignores rank in v1 S5 but we persist it for v2.1 rank-weighted TF-IDF.
//
// Composite primary key (AnimeID, TagID) prevents duplicate joins.
type AnimeTag struct {
	AnimeID   string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	TagID     string    `gorm:"size:200;primaryKey" json:"tag_id"`
	Rank      int       `gorm:"default:0" json:"rank"`
	CreatedAt time.Time `json:"created_at"`
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
	// SourceTypeRaw — raw Japanese audio resolved via the AllAnime parser
	// (workstream raw-jp). Phase 01: AllAnime Parser.
	SourceTypeRaw SourceType = "raw"
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

// KodikStreamSource is the decoded, ad-free HLS stream for a Kodik episode.
// Unlike KodikVideoSource (an iframe embed link), this carries a direct .m3u8
// URL that the frontend proxies through /api/streaming/hls-proxy.
type KodikStreamSource struct {
	StreamURL     string `json:"stream_url"` // raw .m3u8 on the Kodik CDN
	Referer       string `json:"referer"`    // Referer to send to the CDN
	Quality       int    `json:"quality"`    // chosen quality
	Qualities     []int  `json:"qualities"`  // all available qualities
	Episode       int    `json:"episode"`
	TranslationID int    `json:"translation_id"`
	Translation   string `json:"translation"`
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
	Query     string
	Year      *int
	YearFrom  *int
	YearTo    *int
	Season    string
	Status    AnimeStatus
	GenreIDs  []string
	StudioIDs []string
	Sort      string
	Order     string
	Page      int
	PageSize  int
	Source    string
	// Phase 15 (UX-31) — multi-axis browse sidebar filters.
	// Kinds is the OR-set of Shikimori kinds: "tv" / "movie" / "ova" /
	// "ona" / "special" / "tv_special" / "music" / "cm" / "pv". A row
	// passes when its kind matches ANY selected value. Empty = no filter.
	// Providers is the OR-set of {"kodik","dub","raw","ae"} → columns
	// has_kodik/has_dub/has_raw/has_video — a row passes when ANY of the
	// selected columns is true. Empty = no filter. Unknown values dropped at
	// the handler. StudioIDs is an OR-set over the anime_studios join.
	Kinds     []string
	Providers []string
	// ScoreMin filters to anime with score >= this value. nil = no filter.
	ScoreMin *float64
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

// HanimeEpisode represents an episode from Hanime.
type HanimeEpisode struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// HanimeSource represents a single video quality source from Hanime.
type HanimeSource struct {
	URL    string  `json:"url"`
	Height string  `json:"height"`
	Width  int     `json:"width"`
	SizeMB float64 `json:"size_mb"`
}

// HanimeStream represents stream data from Hanime.
type HanimeStream struct {
	Sources []HanimeSource `json:"sources"`
}

// Anime18Episode represents one episode from the 18anime.me provider.
type Anime18Episode struct {
	Slug   string `json:"slug"`   // full slug incl. numeric id, e.g. "1167-...-episode-2"
	URL    string `json:"url"`    // canonical episode page URL
	Number int    `json:"number"` // 1-based episode number
}

// Anime18Stream is a resolved playable source from an 18anime embed mirror.
type Anime18Stream struct {
	URL     string `json:"url"`               // direct mp4 or m3u8 URL
	Referer string `json:"referer,omitempty"` // Referer the HLS proxy must inject ("" if none)
	IsHLS   bool   `json:"is_hls"`            // true => m3u8 (turbovid), false => progressive mp4 (mp4upload)
	Quality string `json:"quality"`           // e.g. "FullHD"
}

// AnimeLib types

// AnimeLibEpisode represents an episode from AnimeLib
type AnimeLibEpisode struct {
	ID     int    `json:"id"`
	Number string `json:"number"`
	Name   string `json:"name"`
}

// AnimeLibTranslation represents an available translation/dubbing from AnimeLib
type AnimeLibTranslation struct {
	ID           int    `json:"id"`
	TeamName     string `json:"team_name"`
	Type         string `json:"type"`          // "voice" or "subtitles"
	Player       string `json:"player"`        // "Animelib" (direct video) or "Kodik" (iframe)
	HasSubtitles bool   `json:"has_subtitles"` // true if external subtitle files exist
}

// AnimeLibSource represents a single video quality source from AnimeLib
type AnimeLibSource struct {
	URL     string `json:"url"`
	Quality int    `json:"quality"` // 360, 720, 1080, 2160
}

// AnimeLibSubtitle represents an external subtitle file from AnimeLib
type AnimeLibSubtitle struct {
	Format string `json:"format"` // "ass", "vtt"
	URL    string `json:"url"`
}

// AnimeLibStream represents stream source data from AnimeLib
type AnimeLibStream struct {
	Sources   []AnimeLibSource   `json:"sources,omitempty"`   // direct MP4 video sources (Animelib player)
	Subtitles []AnimeLibSubtitle `json:"subtitles,omitempty"` // external subtitle files (ASS, VTT)
}

// AnimeLibSearchResult represents a search result from AnimeLib
type AnimeLibSearchResult struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	RusName string `json:"rus_name"`
	Poster  string `json:"poster"`
	SlugURL string `json:"slug_url"`
}
