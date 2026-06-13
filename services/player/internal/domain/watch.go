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
	// Authoritative external IDs from the catalog-owned animes row (Shikimori ID == MAL ID).
	// Read-only here; the player service mirrors these columns for export/lookup.
	ShikimoriID string      `gorm:"column:shikimori_id" json:"shikimori_id,omitempty"`
	MALID       string      `gorm:"column:mal_id" json:"mal_id,omitempty"`
	Genres      []GenreInfo `gorm:"many2many:anime_genres;joinForeignKey:anime_id;joinReferences:genre_id" json:"genres,omitempty"`
}

func (AnimeInfo) TableName() string { return "animes" }

// MaxRewatchCount is the upper clamp for a manually-edited rewatch_count
// (abuse guard). Design 2026-06-05.
const MaxRewatchCount = 9999

type WatchProgress struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        string    `gorm:"type:uuid;uniqueIndex:idx_watch_progress_user_anime_ep,priority:1" json:"user_id"`
	AnimeID       string    `gorm:"type:uuid;uniqueIndex:idx_watch_progress_user_anime_ep,priority:2" json:"anime_id"`
	EpisodeNumber int       `gorm:"uniqueIndex:idx_watch_progress_user_anime_ep,priority:3" json:"episode_number"`
	Progress      int       `json:"progress"`
	Duration      int       `json:"duration"`
	Completed     bool      `gorm:"default:false" json:"completed"`
	// WatchCount is incremented every time MarkCompleted is called on a row
	// that already has completed=true. 1 = first watch, 2+ = rewatch. Phase 5
	// gap-fill (G-02) — Tier 2 inference uses this to detect rewatch behavior
	// and avoid letting a single binge skew "what does this user usually pick"
	// for a much-longer-watched series.
	WatchCount int `gorm:"default:1" json:"watch_count"`
	// DroppedOffAt records the seconds-into-episode where the user closed the
	// page without completing. Populated by the dropoff beacon endpoint when
	// the user navigates away mid-episode. NULL when the episode was completed
	// or never abandoned. Phase 5 gap-fill (G-01).
	DroppedOffAt  *int      `json:"dropped_off_at,omitempty"`
	LastWatchedAt time.Time `json:"last_watched_at"`
	CreatedAt     time.Time `json:"created_at"`
	// UpdatedAt carries a dedicated B-tree index (HSB-NF-02) so the
	// workstream hero-spotlight v1.0 Phase 3 `now_watching` resolver's
	// `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'` predicate runs
	// against an index instead of degrading to a sequential scan as
	// watch_progress grows. AutoMigrate creates the index on player restart.
	UpdatedAt time.Time `gorm:"index:idx_watch_progress_updated_at" json:"updated_at"`
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
	// ReviewText / Username — Phase 1 (workstream: social). These columns
	// absorb the legacy `reviews` table so a single anime_list row carries
	// both the user's score AND their optional written review. NOT NULL with
	// '' default so legacy rows remain valid pre-migration. See
	// cmd/player-api/main.go runSocialMigration helper.
	ReviewText   string     `gorm:"type:text;not null;default:''" json:"review_text"`
	Username     string     `gorm:"size:32;not null;default:''" json:"username"`
	IsRewatching bool       `gorm:"default:false" json:"is_rewatching"`
	// RewatchCount = number of COMPLETED rewatches (MAL "times rewatched").
	// Auto-incremented when a watching→completed transition occurs while
	// IsRewatching is true; also settable manually and via import. Design
	// 2026-06-05. Total times watched = 1 + RewatchCount.
	RewatchCount int        `gorm:"default:0" json:"rewatch_count"`
	Priority     string     `gorm:"size:20" json:"priority"`
	MalID        *int       `json:"mal_id,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Reactions is a TRANSIENT (non-persisted) projection of this row's emoji
	// reactions, populated by ReviewService for the review endpoints. `gorm:"-"`
	// keeps it out of all SQL; `json:"-"` keeps it off the watchlist wire shape
	// (the review handler copies it into reviewResponse.Reactions). AUTO-408.
	Reactions []ReactionCount `gorm:"-" json:"-"`
	// UserAvatar is a TRANSIENT (non-persisted) copy of the author's CURRENT
	// avatar from the users table, populated at read time by the review
	// queries (same pattern as ActivityEvent.UserAvatar). `json:"-"` keeps it
	// off the watchlist wire shape — the review handler projects it into
	// reviewResponse.UserAvatar.
	UserAvatar string `gorm:"-" json:"-"`
}

func (AnimeListEntry) TableName() string {
	return "anime_list"
}

// ReviewReaction is a single user's emoji reaction to a review (an anime_list
// row that qualifies as a review). The platform enforces ONE reaction per
// (review, user) at the application layer (toggle = replace-or-remove); the DB
// keeps the legacy (review, user, emoji) unique index until the deferred
// historical multi-reactions are cleaned, after which it can tighten to
// (review, user). Username is denormalized (captured at react time) so the
// "who reacted" popover needs no users-table join. The FK to anime_list(id)
// with ON DELETE CASCADE is enforced via raw SQL in cmd/player-api/main.go
// (GORM does not infer FKs from struct tags alone). AUTO-408.
type ReviewReaction struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ReviewID  string    `gorm:"type:uuid;not null;index;uniqueIndex:idx_review_reaction_unique,priority:1" json:"review_id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:idx_review_reaction_unique,priority:2" json:"user_id"`
	Emoji     string    `gorm:"size:10;not null;uniqueIndex:idx_review_reaction_unique,priority:3" json:"emoji"`
	Username  string    `gorm:"size:64" json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

func (ReviewReaction) TableName() string { return "review_reactions" }

// System reaction identity — the reserved «AnimeEnigma» pseudo-user that owns
// the auto-👍 seeded on admin-authored reviews. NOT a real users-table row;
// the fixed sentinel UUID + denormalized username let it appear in reactions
// (and the who-reacted popover) without impersonating anyone. The
// one-per-(review,user) rule covers it too — it only ever sets 👍 once. AUTO-408.
const (
	SystemReactionUserID   = "00000000-0000-0000-0000-0000a01e0608" // reserved, non-allocatable
	SystemReactionUsername = "AnimeEnigma"
	SystemReactionEmoji    = "👍"
)

// ReactionCount is the per-emoji aggregate attached to a review in API
// responses. ReactedByMe is true when the requesting (authenticated) viewer
// has reacted with this emoji on the review. Users is the ordered list of
// reactor display names (for the Discord/TG-style "who reacted" popover); the
// System reactor appears as «AnimeEnigma». Reactors mirrors Users but carries
// the user IDs too, so the admin moderation UI can target a specific user's
// reaction for removal. AUTO-408.
type ReactionCount struct {
	Emoji       string         `json:"emoji"`
	Count       int            `json:"count"`
	ReactedByMe bool           `json:"reacted_by_me"`
	Users       []string       `json:"users"`
	Reactors    []ReactionUser `json:"reactors"`
}

// ReactionUser identifies a single reactor inside a ReactionCount aggregate
// (admin moderation needs the user ID, not just the display name).
type ReactionUser struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// AllowedReactionEmojis is the fixed 12-emoji palette the review-reaction
// endpoint accepts. Toggling any emoji outside this set is rejected with
// InvalidInput. AUTO-408 (admin @tNeymik request).
var AllowedReactionEmojis = map[string]bool{
	"👍": true, "❤️": true, "🫠": true, "🤮": true,
	"🤧": true, "🤯": true, "🥴": true, "😈": true,
	"🤡": true, "🤩": true, "😏": true, "🥰": true,
}

// WatchHistory records a watched episode with full combo context
type WatchHistory struct {
	ID               string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           string `gorm:"type:uuid;not null;index:idx_wh_user_combo" json:"user_id"`
	AnimeID          string `gorm:"not null;index;index:idx_wh_anime_combo" json:"anime_id"`
	EpisodeNumber    int    `gorm:"not null" json:"episode_number"`
	Player           string `gorm:"size:20;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"player"`
	Language         string `gorm:"size:5;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"language"`
	WatchType        string `gorm:"size:5;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"watch_type"`
	TranslationID    string `gorm:"size:50" json:"translation_id"`
	TranslationTitle string `gorm:"size:200" json:"translation_title"`
	DurationWatched  int    `gorm:"default:0" json:"duration_watched"`
	// SessionID is a UUID generated by the frontend per playback session.
	// Heartbeat saves and the completion mark for the same playback share it,
	// so Tier 2 aggregation can distinguish "fresh open" from "in-session
	// resume". Phase 5 gap-fill (G-04-lite). Empty string for legacy rows.
	SessionID string    `gorm:"size:36;index" json:"session_id"`
	WatchedAt time.Time `gorm:"not null;default:now()" json:"watched_at"`
}

func (WatchHistory) TableName() string {
	return "watch_history"
}

// Phase 1 (workstream: social) plan 02 — the legacy `Review` struct + its
// `TableName()` method are GONE. The `reviews` table was merged into
// `anime_list` by Plan 01's runSocialMigration; Plan 02 finished the
// cleanup by removing the Go type and the repository that owned it. The
// CreateReviewRequest DTO below STAYS — it's still the POST body shape
// consumed by handler/review.go (CreateOrUpdateReview).

// Request/Response types (not database tables)
type UpdateProgressRequest struct {
	AnimeID          string `json:"anime_id"`
	EpisodeNumber    int    `json:"episode_number"`
	Progress         int    `json:"progress"`
	Duration         int    `json:"duration"`
	Player           string `json:"player,omitempty"`
	Language         string `json:"language,omitempty"`
	WatchType        string `json:"watch_type,omitempty"`
	TranslationID    string `json:"translation_id,omitempty"`
	TranslationTitle string `json:"translation_title,omitempty"`
	// SessionID — frontend playback session UUID; correlates heartbeat saves
	// with the eventual completion event in WatchHistory. Phase 5 (G-04-lite).
	SessionID string `json:"session_id,omitempty"`
}

// MarkEpisodeWatchedRequest extends the episode-watched payload with combo context
type MarkEpisodeWatchedRequest struct {
	Episode          int    `json:"episode"`
	Player           string `json:"player,omitempty"`
	Language         string `json:"language,omitempty"`
	WatchType        string `json:"watch_type,omitempty"`
	TranslationID    string `json:"translation_id,omitempty"`
	TranslationTitle string `json:"translation_title,omitempty"`
	// SessionID — frontend playback session UUID. Persisted on the
	// WatchHistory row for this completion. Phase 5 (G-04-lite).
	SessionID string `json:"session_id,omitempty"`
}

// DropOffRequest is the body of the dropoff beacon — sent by the player when
// the user navigates away without completing. The frontend uses navigator.sendBeacon
// so the request must be small and self-contained. Phase 5 (G-01).
type DropOffRequest struct {
	EpisodeNumber int    `json:"episode_number"`
	Progress      int    `json:"progress"`
	SessionID     string `json:"session_id,omitempty"`
}

type UpdateListRequest struct {
	AnimeID      string     `json:"anime_id"`
	Status       string     `json:"status"`
	Score        *int       `json:"score,omitempty"`
	Episodes     *int       `json:"episodes,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	Tags         *string    `json:"tags,omitempty"`
	IsRewatching *bool      `json:"is_rewatching,omitempty"`
	// RewatchCount — manual edit of completed-rewatch tally. nil = leave
	// untouched (PATCH). Clamped to [0, MaxRewatchCount]. Design 2026-06-05.
	RewatchCount *int       `json:"rewatch_count,omitempty"`
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
		"episodes":   true,
		"title":      true,
		"genre":      true,
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

// WatchlistStats contains aggregate stats for a user's watchlist
type WatchlistStats struct {
	AvgScore      float64 `json:"avg_score"`
	TotalEpisodes int     `json:"total_episodes"`
	TotalEntries  int     `json:"total_entries"`
	Completed     int     `json:"completed"`
}

// ContinueWatchingItem is the per-row payload of GET /users/continue-watching.
// One item per anime — the user's most-recent in-progress episode for that
// anime, with the AnimeInfo projection inlined for poster/title rendering.
// Phase 8 (UX-15 / UA-061).
type ContinueWatchingItem struct {
	Anime         AnimeInfo `json:"anime"`
	EpisodeNumber int       `json:"episode_number"`
	Progress      int       `json:"progress"`
	Duration      int       `json:"duration"`
	LastWatchedAt time.Time `json:"last_watched_at"`
	DroppedOffAt  *int      `json:"dropped_off_at,omitempty"`
}

// BulkAnimeProgressEntry is the per-anime payload of GET
// /users/anime-progress. Aggregates the user's watch state for one anime so
// the frontend can render a single progress badge per card. Phase 9 (UX-16).
type BulkAnimeProgressEntry struct {
	LatestEpisode int  `json:"latest_episode"` // highest episode_number with any row for this user+anime
	EpisodesCount int  `json:"episodes_count"` // from animes.episodes_count
	EpisodesAired int  `json:"episodes_aired"` // from animes.episodes_aired
	Completed     bool `json:"completed"`      // true when latest_episode row.completed=true AND user reached at least episodes_count (or episodes_aired when count unknown)
	Dropped       bool `json:"dropped"`        // true when the latest row has dropped_off_at != NULL and is not completed
}

// BulkAnimeProgressMap is the response shape — a JSON object keyed by
// anime_id (string) -> BulkAnimeProgressEntry. Animes the user has no
// progress on are omitted from the map; the frontend treats absence as
// "no badge". Phase 9 (UX-16).
type BulkAnimeProgressMap map[string]BulkAnimeProgressEntry

// InternalListItem is the per-anime payload of GET /internal/users/{user_id}/list.
// Used by the catalog spotlight aggregator (workstream hero-spotlight v1.0 Phase 3
// — not_time_yet + continue_watching_new resolvers). Joined from anime_list + animes
// + watch_progress in one query so callers do not fan out per anime.
//
// Trust boundary: this struct crosses the docker-network only — the
// /internal/* route is NOT proxied by the gateway, and no JWT is required.
// The catalog resolver passes a JWT-derived user_id; no untrusted user input
// reaches this surface.
type InternalListItem struct {
	AnimeID            string `json:"anime_id"`
	Name               string `json:"name,omitempty"`
	NameRU             string `json:"name_ru,omitempty"`
	PosterURL          string `json:"poster_url,omitempty"`
	EpisodesAired      int    `json:"episodes_aired,omitempty"`
	EpisodesCount      int    `json:"episodes_count,omitempty"`
	Status             string `json:"status"`                         // "watching" | "planned" | "postponed"
	LastWatchedEpisode int    `json:"last_watched_episode,omitempty"` // 0 when no watch_progress row
	UpdatedAt          string `json:"updated_at,omitempty"`           // anime_list.updated_at (ISO 8601)
}

// AnimeStatusEntry is a lightweight entry for the status map and stats
type AnimeStatusEntry struct {
	AnimeID  string `json:"anime_id" gorm:"column:anime_id"`
	Status   string `json:"status" gorm:"column:status"`
	Score    int    `json:"score" gorm:"column:score"`
	Episodes int    `json:"episodes" gorm:"column:episodes"`
}

// ListFilters holds the optional watchlist filter dimensions added 2026-06-13.
// All fields are zero-value-safe: an empty ListFilters applies no filtering.
type ListFilters struct {
	GenreIDs []string // AND semantics — an anime must carry ALL listed genres
	Kinds    []string // OR semantics — animes.kind IN (Kinds)
	YearMin  *int     // nil = open lower bound
	YearMax  *int     // nil = open upper bound
}

// IsEmpty reports whether no filter dimension is set.
func (f ListFilters) IsEmpty() bool {
	return len(f.GenreIDs) == 0 && len(f.Kinds) == 0 && f.YearMin == nil && f.YearMax == nil
}

// KnownKinds is the validation whitelist for the `kind` filter param. Mirrors
// the distinct animes.kind values present in the catalog.
var KnownKinds = map[string]bool{
	"tv": true, "movie": true, "ova": true, "ona": true, "special": true,
	"tv_special": true, "music": true, "cm": true, "pv": true,
}

// FacetGenre is one genre option for the watchlist filter UI, with the count of
// the user's list entries carrying that genre.
type FacetGenre struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NameRU string `json:"name_ru"`
	Count  int64  `json:"count"`
}

// FacetKind is one type/kind option with its count.
type FacetKind struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

// FacetYearRange is the min/max release year present in the user's list.
// Both nil when the list has no entries with a known (non-zero) year.
type FacetYearRange struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

// ListFacets is the response of the watchlist facets endpoint.
type ListFacets struct {
	Genres []FacetGenre   `json:"genres"`
	Kinds  []FacetKind    `json:"kinds"`
	Years  FacetYearRange `json:"years"`
}

// ValidListStatuses is the whitelist of watchlist status values accepted by
// bulk operations. Mirrors the per-status filter pills in the profile UI.
var ValidListStatuses = map[string]bool{
	"watching": true, "plan_to_watch": true, "completed": true,
	"on_hold": true, "dropped": true,
}

// BulkUpdateRequest is the body of POST /users/watchlist/bulk.
// Action is "set_status" (requires Status) or "remove".
type BulkUpdateRequest struct {
	AnimeIDs []string `json:"anime_ids"`
	Action   string   `json:"action"`
	Status   string   `json:"status,omitempty"`
}
