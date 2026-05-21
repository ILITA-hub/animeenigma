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

// randomTailPage is the repo page index for ranks 201..300 with
// PageSize=100. Combined with Sort="score" Order="desc", this fetches
// the third hundred by (sort_priority DESC, score DESC) — see GOTCHA
// comment inside Resolve. Page 3 (not 2) so the pool does not overlap
// `anime_of_day`'s top-200 pool, which previously caused both cards to
// surface the same anime on date-seed collisions (e.g. Kimetsu no Yaiba
// at rank 180 — well-known title users perceive as "top 100").
const randomTailPage = 3

// randomTailPageSize is the candidate pool — 100 anime ranked 201..300.
const randomTailPageSize = 100

// RandomTailResolver implements spotlight.Resolver for the
// `random_tail` card. Picks one anime per UTC calendar day from ranks
// 201..300 by score — "good but not top-rated" discovery surface that
// excludes anime_of_day's top-200 pool.
type RandomTailResolver struct {
	repo  animeSearcher
	cache cache.Cache
	log   *logger.Logger
}

// NewRandomTailResolver constructs the resolver.
func NewRandomTailResolver(repo animeSearcher, c cache.Cache, log *logger.Logger) *RandomTailResolver {
	return &RandomTailResolver{repo: repo, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *RandomTailResolver) Type() string { return "random_tail" }

// Resolve returns the day's random_tail card. userID is ignored — the
// pick is global. Eligibility: empty pool → (nil, nil), no cache write.
func (r *RandomTailResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:random_tail:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.RandomTailData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: compute ---------------------------------------
	//
	// GOTCHA (services/catalog/internal/repo/anime.go:134-147):
	// AnimeRepository.Search injects "sort_priority DESC" as the primary
	// sort axis. Page=3, PageSize=100 returns ranks 201..300 by
	// (sort_priority DESC, score DESC). Pinned anime never appear in
	// discovery (intended per CLAUDE.md "Pinning anime to the top").
	// This is the desired behaviour — pinned items are already prominently
	// surfaced elsewhere, so they don't need a second slot in random_tail.
	items, _, err := r.repo.Search(ctx, domain.SearchFilters{
		Sort:     "score",
		Order:    "desc",
		Page:     randomTailPage,
		PageSize: randomTailPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("random_tail: repo search: %w", err)
	}
	if len(items) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5).
		return nil, nil
	}

	seed := spotlight.DateSeedUTC(time.Now())
	picked := items[seed%len(items)]
	data := spotlight.RandomTailData{Anime: *picked}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
