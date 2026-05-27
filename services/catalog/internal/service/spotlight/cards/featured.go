// Package cards contains the per-card resolvers that implement the
// spotlight.Resolver interface (Plan 01-01 types.go). Phase 1 ships
// four resolvers: featured, random_tail, latest_news, platform_stats.
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

// minScoreFeatured is the lower-bound score filter for the daily pick.
// Per CONTEXT.md decision: only "top-rated" anime are eligible.
const minScoreFeatured = 8.0

// featuredPoolSize is the candidate pool — first 200 anime by
// (sort_priority DESC, score DESC) with score >= 8.0. Date-seeded
// modulus over this slice gives the day's pick.
const featuredPoolSize = 200

// cardTTL is the per-card cache lifetime — one calendar day. Cards
// roll over at UTC midnight via the date suffix in the cache key.
const cardTTL = 24 * time.Hour

// FeaturedResolver implements spotlight.Resolver for the `featured` card.
// An admin-pinned anime (sort_priority > 0) takes precedence over the
// daily-seeded pick from the top-200 highly-rated candidates.
type FeaturedResolver struct {
	repo  animeSearcher
	cache cache.Cache
	log   *logger.Logger
}

// NewFeaturedResolver constructs the resolver. All deps are required;
// nil cache or repo will panic on first Resolve.
func NewFeaturedResolver(repo animeSearcher, c cache.Cache, log *logger.Logger) *FeaturedResolver {
	return &FeaturedResolver{repo: repo, cache: c, log: log}
}

// Type returns the card discriminator string consumed by the frontend's
// TypeScript discriminated union.
func (r *FeaturedResolver) Type() string { return "featured" }

// Resolve returns the featured card. userID is ignored — the pick is
// global (every user sees the same anime on the same UTC day).
//
// Priority: an admin-pinned anime (sort_priority > 0) wins over the
// daily-seeded pick. If no pinned anime exists, falls back to the
// date-seeded pick from the top-rated pool.
//
// Eligibility: card is dropped (returns nil, nil) when both the pinned
// query and the candidate pool are empty (e.g. fresh DB). The (nil, nil)
// path does NOT write to cache (Pitfall 5 from 01-RESEARCH.md).
func (r *FeaturedResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:featured:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.FeaturedData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		// Hard-down Redis or decode error — log and fall through to
		// compute path. We never fail the user request because Redis
		// is sick.
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Curated pin (sort_priority > 0) wins when present --------------
	// The repo orders by sort_priority DESC naturally; request 1 result
	// and verify it actually has sort_priority > 0 (i.e. is truly pinned).
	pinned, _, perr := r.repo.Search(ctx, domain.SearchFilters{
		Sort:     "sort_priority",
		Order:    "desc",
		Page:     1,
		PageSize: 1,
	})
	if perr == nil && len(pinned) > 0 && pinned[0].SortPriority > 0 {
		data := spotlight.FeaturedData{Anime: *pinned[0]}
		if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
			r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
		}
		return &spotlight.Card{Type: r.Type(), Data: data}, nil
	}

	// --- Cache MISS path: daily-seeded pick ----------------------------
	sm := minScoreFeatured
	items, _, err := r.repo.Search(ctx, domain.SearchFilters{
		Sort:     "score",
		Order:    "desc",
		ScoreMin: &sm,
		Page:     1,
		PageSize: featuredPoolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("featured: repo search: %w", err)
	}
	if len(items) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5).
		return nil, nil
	}

	seed := spotlight.DateSeedUTC(time.Now())
	picked := items[seed%len(items)]
	data := spotlight.FeaturedData{Anime: *picked}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
