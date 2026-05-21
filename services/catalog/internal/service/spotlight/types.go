// Package spotlight defines the type system and aggregator for the
// `GET /api/home/spotlight` endpoint (workstream hero-spotlight, v1.0
// Phase 1). The package contracts that resolvers (per-card implementations
// living under spotlight/cards/) and the aggregator (this package) compile
// against.
//
// The JSON shape produced here is load-bearing for the Phase 2 frontend —
// every struct exactly matches the TypeScript discriminated union from
// docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md §4.1.
//
// Plan 01-01 ships types + seed helpers + an Aggregator skeleton. Plan
// 01-02 ships the 4 resolvers. Plan 01-03 replaces the Aggregator stub
// with the concurrent fan-out + snapshot-fallback implementation.
package spotlight

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// ChangelogEntry is one flattened changelog item — the outer per-date
// group from `frontend/web/public/changelog.json` is flattened into one
// ChangelogEntry per inner entry, carrying the outer Date. Plan 02's
// `latest_news` resolver produces a slice of these.
type ChangelogEntry struct {
	Date    string `json:"date"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// Card is the outer discriminated-union envelope. Each resolver produces
// a Card with its own Type discriminator (e.g. "anime_of_day") and a
// per-type Data struct embedded as `any`. The TypeScript side narrows on
// the `type` field.
type Card struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// AnimeOfDayData is the payload for `Card{Type: "anime_of_day"}`.
// ReasonI18nKey is optional — omitted from JSON when empty.
type AnimeOfDayData struct {
	Anime         domain.Anime `json:"anime"`
	ReasonI18nKey string       `json:"reason_i18n_key,omitempty"`
}

// RandomTailData is the payload for `Card{Type: "random_tail"}`.
type RandomTailData struct {
	Anime domain.Anime `json:"anime"`
}

// LatestNewsData is the payload for `Card{Type: "latest_news"}`.
type LatestNewsData struct {
	Entries []ChangelogEntry `json:"entries"`
}

// StatsMetric is one metric inside `PlatformStatsData.Metrics`. Delta is
// a pointer so it can be omitted (nil) when the previous-period value is
// unknown.
type StatsMetric struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
	Delta *int64 `json:"delta,omitempty"`
}

// PlatformStatsData is the payload for `Card{Type: "platform_stats"}`.
type PlatformStatsData struct {
	Metrics []StatsMetric `json:"metrics"`
}

// --- Phase 3 dynamic card payloads -------------------------------------
//
// The five structs below extend the Card discriminated union for the Phase 3
// dynamic resolvers (workstream hero-spotlight v1.0). Each maps 1:1 to the
// TypeScript discriminated union extension in
// docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md §4.1.

// PersonalPickItem is one suggestion in the personal_pick card.
// ReasonI18nKey is optional (omitted from JSON when empty).
type PersonalPickItem struct {
	Anime         domain.Anime `json:"anime"`
	ReasonI18nKey string       `json:"reason_i18n_key,omitempty"`
}

// PersonalPickData is the payload for `Card{Type: "personal_pick"}` —
// HSB-BE-20. Items is 1..3 after AdaptiveSlice (HSB-BE-30). Source is
// "trending" for anon callers, "personal" for logged-in callers.
//
// Items MUST be initialized as `[]PersonalPickItem{}` (NOT a nil slice) so
// it marshals as `"items":[]` — the Phase 2 frontend treats `null` as a
// parse failure (see TestPersonalPickData_ItemsMarshalAsArray).
type PersonalPickData struct {
	Items  []PersonalPickItem `json:"items"`
	Source string             `json:"source"`
}

// TelegramPost is one excerpt in the telegram_news card. Title / Link /
// Date are optional (Telegram channel scrapes sometimes lack each).
type TelegramPost struct {
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt"`
	Link    string `json:"link,omitempty"`
	Date    string `json:"date,omitempty"`
}

// TelegramNewsData is the payload for `Card{Type: "telegram_news"}` —
// HSB-BE-21. Posts is 1..3 after AdaptiveSlice (HSB-BE-30).
type TelegramNewsData struct {
	Posts []TelegramPost `json:"posts"`
}

// NowWatchingSession exposes ONLY public user fields per HSB-NF-04
// (privacy gate). The user UUID NEVER leaves the SQL — only username +
// public_id are projected; both are publicly visible on user profile
// pages already.
type NowWatchingSession struct {
	Username      string `json:"username"`
	PublicID      string `json:"public_id"`
	AnimeID       string `json:"anime_id"`
	AnimeName     string `json:"anime_name,omitempty"`
	AnimeNameRU   string `json:"anime_name_ru,omitempty"`
	PosterURL     string `json:"poster_url,omitempty"`
	EpisodeNumber int    `json:"episode_number"`
	UpdatedAt     string `json:"updated_at"`
}

// NowWatchingData is the payload for `Card{Type: "now_watching"}` —
// HSB-BE-22 + HSB-NF-04. Sessions is 1..3 after AdaptiveSlice (HSB-BE-30).
type NowWatchingData struct {
	Sessions []NowWatchingSession `json:"sessions"`
}

// NotTimeYetData is the payload for `Card{Type: "not_time_yet"}` —
// HSB-BE-24. Single-item card (login only). Status is "planned" or
// "postponed".
type NotTimeYetData struct {
	Anime  domain.Anime `json:"anime"`
	Status string       `json:"status"`
}

// ContinueWatchingNewData is the payload for `Card{Type:
// "continue_watching_new"}` — HSB-BE-25. Single-item card (login only).
// NewEpisodeNumber is the anime's EpisodesAired (the newest aired episode
// number); LastWatchedEpisode is the user's most-recent watch_progress
// episode number. A card is eligible when NewEpisodeNumber > LastWatchedEpisode + 1.
type ContinueWatchingNewData struct {
	Anime              domain.Anime `json:"anime"`
	LastWatchedEpisode int          `json:"last_watched_episode"`
	NewEpisodeNumber   int          `json:"new_episode_number"`
}

// Response is the top-level envelope returned by `GET /api/home/spotlight`.
//
// CRITICAL: Cards MUST marshal as `[]` (empty array) and NOT `null` when
// empty — the Phase 2 frontend treats `null` as a parse failure. Callers
// MUST initialize via `Cards: []Card{}` (NOT `var Cards []Card`) so the
// underlying slice is non-nil. See TestTypes_EmptyCardsMarshalArray for
// the regression guard.
type Response struct {
	Cards       []Card `json:"cards"`
	GeneratedAt string `json:"generated_at"`
}

// Resolver is the contract each per-card resolver implements (Plan 02).
// The aggregator (Plan 03) fans out across all registered resolvers
// concurrently with a per-card 800ms deadline.
//
// Return semantics:
//   - (*Card, nil)  — resolver succeeded, card is eligible; aggregator
//     includes it in the response.
//   - (nil, nil)    — resolver succeeded, card is NOT eligible (no data);
//     aggregator drops it silently, no log line emitted. Use this for
//     "today's pool of candidates is empty" or "metric is unavailable"
//     paths.
//   - (nil, err)    — resolver failed (timeout, upstream error, etc.);
//     aggregator drops the card AND emits a structured log line
//     `spotlight.card_failed{type, error}` via libs/logger.Errorw.
type Resolver interface {
	Type() string
	Resolve(ctx context.Context, userID *string) (*Card, error)
}
