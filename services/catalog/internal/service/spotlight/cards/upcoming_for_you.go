// UpcomingForYouResolver implements spotlight.Resolver for the
// `upcoming_for_you` card (spec 2026-07-17). Login-only — anon callers get
// (nil, nil) BEFORE any fetch. Calls recs GET /api/users/recs/upcoming with
// the caller's JWT (jwt_context.go pattern); recs owns matching + its own
// 6h per-user cache, so the resolver adds only a short 60s cache to absorb
// refresh-mashing. Empty matches → ineligible (nil, nil) → the slide simply
// doesn't render, which keeps the surface rare by construction.
//
// Product rule (2026-07-18): an announcement the user has ALREADY added to
// their list must never surface here. recs excludes anime_list entries at
// COMPUTE time, but its 6h cache — and this resolver's 60s cache — can
// predate an add, so a cached snapshot may still carry a just-added title.
// The resolver therefore re-filters the item set against anime_list LIVE on
// every resolve (dropAlreadyListed): self-healing source-of-truth check, no
// cross-service cache invalidation, honoured within one 60s TTL at worst.

package cards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

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

// listedFilter reports which of the given anime IDs the user already has in
// their list. An interface so tests substitute a fake without a real
// *gorm.DB (now_watching.go's gormNowWatchingAdapter precedent).
type listedFilter interface {
	ListedAnimeIDs(ctx context.Context, userID string, animeIDs []string) (map[string]struct{}, error)
}

const (
	upcomingForYouKeyPrefix = "spotlight:upcoming_for_you:"
	upcomingForYouTTL       = 60 * time.Second
)

// UpcomingForYouResolver resolves the upcoming_for_you card.
type UpcomingForYouResolver struct {
	recs   upcomingFetcher
	listed listedFilter
	cache  cache.Cache
	log    *logger.Logger
}

// NewUpcomingForYouResolver constructs the resolver.
func NewUpcomingForYouResolver(recs upcomingFetcher, listed listedFilter, c cache.Cache, log *logger.Logger) *UpcomingForYouResolver {
	return &UpcomingForYouResolver{recs: recs, listed: listed, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *UpcomingForYouResolver) Type() string { return "upcoming_for_you" }

// Resolve produces the card, or (nil, nil) when the user is anonymous, the
// JWT is missing (defensive), or no matches remain after dropping titles the
// user already has in their list.
func (r *UpcomingForYouResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	if userID == nil || *userID == "" {
		return nil, nil
	}
	jwt, ok := JWTFromContext(ctx)
	if !ok {
		return nil, nil
	}
	key := upcomingForYouKeyPrefix + *userID

	// Load the recs matching snapshot from cache, else fetch + cache it. The
	// already-listed filter runs LIVE afterwards (below), so a title added
	// after the snapshot was cached is still honoured within this TTL.
	var data spotlight.UpcomingForYouData
	if err := r.cache.Get(ctx, key, &data); err != nil {
		if !errors.Is(err, cache.ErrNotFound) {
			r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
		}
		wire, ferr := r.recs.FetchUpcoming(ctx, jwt)
		if ferr != nil {
			return nil, fmt.Errorf("upcoming_for_you: recs fetch: %w", ferr)
		}
		data = spotlight.UpcomingForYouData{Items: []spotlight.UpcomingForYouItem{}}
		for _, it := range wire {
			data.Items = append(data.Items, spotlight.UpcomingForYouItem{
				Anime:      it.Anime,
				MatchScore: it.MatchScore,
				Reason:     it.Reason,
			})
		}
		// Cache the recs snapshot (incl. empty) to absorb refresh-mashing.
		if err := r.cache.Set(ctx, key, data, upcomingForYouTTL); err != nil {
			r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
		}
	}

	data.Items = r.dropAlreadyListed(ctx, *userID, data.Items)
	if len(data.Items) == 0 {
		return nil, nil
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// dropAlreadyListed removes items for announced titles already in the user's
// list. Anime is a verbatim recs json.RawMessage, so we peek only at its
// "id" — no reshaping (personal_pick precedent). Fails OPEN: any decode/query
// error keeps the items (recs already excluded list entries at compute time,
// so the only exposure is the bounded stale-cache window) and never blanks
// the card.
func (r *UpcomingForYouResolver) dropAlreadyListed(ctx context.Context, userID string, items []spotlight.UpcomingForYouItem) []spotlight.UpcomingForYouItem {
	if len(items) == 0 {
		return items
	}
	// Decode each item's id once; "" marks an undecodable item, kept as-is.
	itemIDs := make([]string, len(items))
	toCheck := make([]string, 0, len(items))
	for i, it := range items {
		var a struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(it.Anime, &a); err == nil && a.ID != "" {
			itemIDs[i] = a.ID
			toCheck = append(toCheck, a.ID)
		}
	}
	if len(toCheck) == 0 {
		return items
	}
	listed, err := r.listed.ListedAnimeIDs(ctx, userID, toCheck)
	if err != nil {
		r.log.Warnw("spotlight.upcoming_listed_filter_failed", "type", r.Type(), "error", err)
		return items
	}
	if len(listed) == 0 {
		return items
	}
	out := make([]spotlight.UpcomingForYouItem, 0, len(items))
	for i, it := range items {
		if id := itemIDs[i]; id != "" {
			if _, seen := listed[id]; seen {
				continue // already in the user's list → hide
			}
		}
		out = append(out, it)
	}
	return out
}

// gormListedFilter is the production listedFilter backed by the shared GORM
// connection. Wired in catalog main.go via NewGormListedFilter, mirroring
// NewGormNowWatchingAdapter.
type gormListedFilter struct {
	db *gorm.DB
}

// NewGormListedFilter constructs the production listedFilter.
func NewGormListedFilter(db *gorm.DB) listedFilter {
	return &gormListedFilter{db: db}
}

// ListedAnimeIDs returns the subset of animeIDs present in the user's
// anime_list, as a set. The user_id NEVER leaves this SQL.
func (g *gormListedFilter) ListedAnimeIDs(ctx context.Context, userID string, animeIDs []string) (map[string]struct{}, error) {
	if len(animeIDs) == 0 {
		return nil, nil
	}
	var found []string
	if err := g.db.WithContext(ctx).
		Table("anime_list").
		Where("user_id = ? AND anime_id IN ?", userID, animeIDs).
		Pluck("anime_id", &found).Error; err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(found))
	for _, id := range found {
		set[id] = struct{}{}
	}
	return set, nil
}
