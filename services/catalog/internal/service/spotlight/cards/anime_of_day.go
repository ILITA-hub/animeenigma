// Package cards contains the per-card resolvers that implement the
// spotlight.Resolver interface (Plan 01-01 types.go). Phase 1 ships
// four resolvers: anime_of_day, random_tail, latest_news, platform_stats.
//
// DELIBERATE DIVERGENCE 1 (workstream-wide, baked into acceptance):
// resolvers use manual `cache.Get` + `errors.Is(err, cache.ErrNotFound)` +
// `cache.Set` rather than the convenience helper. Reason: the convenience
// helper would cache nil/empty zero values for the full 24h TTL, baking
// in a "no data" cache that masks an upstream recovery. With manual
// discipline, an empty-result path returns `(nil, nil)` WITHOUT writing
// the cache — so the next request retries until real data appears.
package cards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// animeSearcher is the subset of repo.AnimeRepository that the static
// anime-pick resolvers need. Defined locally so tests can substitute a
// handwritten fake (pattern: services/catalog/internal/service/scraper_test.go).
type animeSearcher interface {
	Search(ctx context.Context, f domain.SearchFilters) ([]*domain.Anime, int64, error)
}

// minScoreAnimeOfDay is the lower-bound score filter for the daily pick.
// Per CONTEXT.md decision: only "top-rated" anime are eligible.
const minScoreAnimeOfDay = 8.0

// animeOfDayPoolSize is the candidate pool — first 200 anime by
// (sort_priority DESC, score DESC) with score >= 8.0. Date-seeded
// modulus over this slice gives the day's pick.
const animeOfDayPoolSize = 200

// cardTTL is the per-card cache lifetime — one calendar day. Cards
// roll over at UTC midnight via the date suffix in the cache key.
const cardTTL = 24 * time.Hour

// AnimeOfDayResolver implements spotlight.Resolver for the
// `anime_of_day` card. Picks one anime per UTC calendar day from the
// top-200 highly-rated candidates by repo.Search ordering.
type AnimeOfDayResolver struct {
	repo  animeSearcher
	cache cache.Cache
	log   *logger.Logger
}

// NewAnimeOfDayResolver constructs the resolver. All deps are required;
// nil cache or repo will panic on first Resolve.
func NewAnimeOfDayResolver(repo animeSearcher, c cache.Cache, log *logger.Logger) *AnimeOfDayResolver {
	return &AnimeOfDayResolver{repo: repo, cache: c, log: log}
}

// Type returns the card discriminator string consumed by the frontend's
// TypeScript discriminated union.
func (r *AnimeOfDayResolver) Type() string { return "anime_of_day" }

// Resolve returns the day's anime_of_day card. userID is ignored — the
// pick is global (every user sees the same anime on the same UTC day).
//
// Eligibility: card is dropped (returns nil, nil) when the candidate
// pool is empty (e.g. fresh DB with no animes >= 8.0). The (nil, nil)
// path does NOT write to cache (Pitfall 5 from 01-RESEARCH.md).
func (r *AnimeOfDayResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:anime_of_day:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.AnimeOfDayData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		// Hard-down Redis or decode error — log and fall through to
		// compute path. We never fail the user request because Redis
		// is sick.
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: compute ---------------------------------------
	sm := minScoreAnimeOfDay
	items, _, err := r.repo.Search(ctx, domain.SearchFilters{
		Sort:     "score",
		Order:    "desc",
		ScoreMin: &sm,
		Page:     1,
		PageSize: animeOfDayPoolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("anime_of_day: repo search: %w", err)
	}
	if len(items) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5).
		return nil, nil
	}

	seed := spotlight.DateSeedUTC(time.Now())
	picked := items[seed%len(items)]
	data := spotlight.AnimeOfDayData{Anime: *picked}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
