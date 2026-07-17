// UpcomingForYouResolver implements spotlight.Resolver for the
// `upcoming_for_you` card (spec 2026-07-17). Login-only — anon callers get
// (nil, nil) BEFORE any fetch. Calls recs GET /api/users/recs/upcoming with
// the caller's JWT (jwt_context.go pattern); recs owns matching + its own
// 6h per-user cache, so the resolver adds only a short 60s cache to absorb
// refresh-mashing. Empty matches → ineligible (nil, nil) → the slide simply
// doesn't render, which keeps the surface rare by construction.

package cards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// upcomingFetcher is the minimal surface this resolver needs; the
// production *client.RecsClient satisfies it implicitly.
type upcomingFetcher interface {
	FetchUpcoming(ctx context.Context, jwt string) ([]client.UpcomingWireItem, error)
}

const (
	upcomingForYouKeyPrefix = "spotlight:upcoming_for_you:"
	upcomingForYouTTL       = 60 * time.Second
)

// UpcomingForYouResolver resolves the upcoming_for_you card.
type UpcomingForYouResolver struct {
	recs  upcomingFetcher
	cache cache.Cache
	log   *logger.Logger
}

// NewUpcomingForYouResolver constructs the resolver.
func NewUpcomingForYouResolver(recs upcomingFetcher, c cache.Cache, log *logger.Logger) *UpcomingForYouResolver {
	return &UpcomingForYouResolver{recs: recs, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *UpcomingForYouResolver) Type() string { return "upcoming_for_you" }

// Resolve produces the card, or (nil, nil) when the user is anonymous, the
// JWT is missing (defensive), or there are no matches.
func (r *UpcomingForYouResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	if userID == nil || *userID == "" {
		return nil, nil
	}
	jwt, ok := JWTFromContext(ctx)
	if !ok {
		return nil, nil
	}
	key := upcomingForYouKeyPrefix + *userID

	var cached spotlight.UpcomingForYouData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		if len(cached.Items) == 0 {
			return nil, nil
		}
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	wire, err := r.recs.FetchUpcoming(ctx, jwt)
	if err != nil {
		return nil, fmt.Errorf("upcoming_for_you: recs fetch: %w", err)
	}

	data := spotlight.UpcomingForYouData{Items: []spotlight.UpcomingForYouItem{}}
	for _, it := range wire {
		data.Items = append(data.Items, spotlight.UpcomingForYouItem{
			Anime:      it.Anime,
			MatchScore: it.MatchScore,
			Reason:     it.Reason,
		})
	}
	// Cache the empty result too — absorbs refresh-mashing for users with
	// no matches (the common case).
	if err := r.cache.Set(ctx, key, data, upcomingForYouTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	if len(data.Items) == 0 {
		return nil, nil
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
