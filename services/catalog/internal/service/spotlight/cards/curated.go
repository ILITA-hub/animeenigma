package cards

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// curatedPriority is the display weight the curated card carries. The
// aggregator leaves any non-zero priority untouched; the frontend uses it
// as the weight in the carousel's random opening-slide pick (1.5× the
// default-1.0 cards).
const curatedPriority = 1.5

// animeGetter is the subset of repo.AnimeRepository the curated resolver
// needs — a single fetch by Shikimori ID (the catalog's primary key).
type animeGetter interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}

// CuratedResolver implements spotlight.Resolver for the `curated` card — a
// single hand-picked anime surfaced ONLY while it is currently airing.
type CuratedResolver struct {
	repo        animeGetter
	cache       cache.Cache
	log         *logger.Logger
	shikimoriID string
}

// NewCuratedResolver constructs the resolver. shikimoriID is the pinned
// Shikimori ID (env SPOTLIGHT_CURATED_SHIKIMORI_ID); an empty string
// disables the card entirely.
func NewCuratedResolver(repo animeGetter, c cache.Cache, log *logger.Logger, shikimoriID string) *CuratedResolver {
	return &CuratedResolver{repo: repo, cache: c, log: log, shikimoriID: shikimoriID}
}

// Type returns the card discriminator consumed by the frontend union.
func (r *CuratedResolver) Type() string { return "curated" }

// Resolve returns the curated card. userID is ignored — the pick is global.
//
// Eligibility (each returns (nil, nil) WITHOUT caching, so an upstream
// recovery is retried on the next request — the workstream's manual-cache
// discipline):
//   - shikimoriID empty (card disabled);
//   - the anime is not in the catalog yet;
//   - the anime is not currently airing (status != ongoing) — this is the
//     entire "active this season only" self-expiry.
func (r *CuratedResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	if r.shikimoriID == "" {
		return nil, nil
	}

	key := "spotlight:curated:" + spotlight.DateKeyUTC(time.Now())

	var cached spotlight.CuratedData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Priority: curatedPriority, Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	anime, err := r.repo.GetByShikimoriID(ctx, r.shikimoriID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, nil // not populated yet — do NOT cache
	}
	if anime.Status != domain.StatusOngoing {
		return nil, nil // finished / announced — self-expire, do NOT cache
	}

	data := spotlight.CuratedData{Anime: *anime}
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Priority: curatedPriority, Data: data}, nil
}
