// Workstream hero-spotlight v1.0 Phase 3 — Plan 03-03 Task 4 (Part A).
//
// NotTimeYetResolver implements spotlight.Resolver for the `not_time_yet`
// card (HSB-BE-24). Login-only — anon callers always get (nil, nil) BEFORE
// any data fetch (T-03-11 info-disclosure mitigation).
//
// Resolves the user's anime_list filtered to statuses ["planned",
// "postponed"], filters items where the show has started airing
// (EpisodesAired > 0), and randomly picks ONE item. Returns ineligible
// (nil, nil) when no item matches.
//
// Cache key spotlight:not_time_yet:<user_id>, TTL 30 seconds.

package cards

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// listByStatusesFetcher is the minimal surface both login-only resolvers
// (not_time_yet, continue_watching_new) need. The production
// *client.PlayerClient satisfies this implicitly.
type listByStatusesFetcher interface {
	FetchListByStatuses(ctx context.Context, userID string, statuses []string) ([]client.InternalListItem, error)
}

// notTimeYetKeyPrefix produces cache keys like "spotlight:not_time_yet:<uid>".
const notTimeYetKeyPrefix = "spotlight:not_time_yet:"

// notTimeYetTTL — 30s is short enough that "I just added it to planned"
// shows up quickly; long enough to absorb refresh-button mashing.
const notTimeYetTTL = 30 * time.Second

// NotTimeYetResolver implements spotlight.Resolver for `not_time_yet`.
type NotTimeYetResolver struct {
	player listByStatusesFetcher
	cache  cache.Cache
	rng    *rand.Rand
	log    *logger.Logger
}

// NewNotTimeYetResolver constructs the resolver. rng may be nil — a
// time-seeded source is provided.
func NewNotTimeYetResolver(p listByStatusesFetcher, c cache.Cache, rng *rand.Rand, log *logger.Logger) *NotTimeYetResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &NotTimeYetResolver{player: p, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *NotTimeYetResolver) Type() string { return "not_time_yet" }

// Resolve produces the not_time_yet card. Anon (userID nil/empty) returns
// (nil, nil) BEFORE any data fetch — login-only contract per T-03-11.
func (r *NotTimeYetResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	if userID == nil || *userID == "" {
		return nil, nil
	}
	key := notTimeYetKeyPrefix + *userID

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.NotTimeYetData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: fetch list ------------------------------------
	items, err := r.player.FetchListByStatuses(ctx, *userID, []string{"planned", "postponed"})
	if err != nil {
		return nil, fmt.Errorf("not_time_yet: list: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Filter to airing items (EpisodesAired > 0).
	eligible := make([]client.InternalListItem, 0, len(items))
	for _, it := range items {
		if it.EpisodesAired > 0 {
			eligible = append(eligible, it)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}

	// Random pick exactly one — the card is single-item by design (HSB-BE-24).
	picked := eligible[r.rng.Intn(len(eligible))]
	anime := domain.Anime{
		ID:            picked.AnimeID,
		Name:          picked.Name,
		NameRU:        picked.NameRU,
		PosterURL:     picked.PosterURL,
		EpisodesAired: picked.EpisodesAired,
		EpisodesCount: picked.EpisodesCount,
	}

	data := spotlight.NotTimeYetData{Anime: anime, Status: picked.Status}
	// Surface anime_list.updated_at as AddedAt (HSB-V11-NT-01). The player
	// client ships it as an RFC3339 string; parse defensively — a missing
	// or malformed timestamp leaves AddedAt nil (omitempty drops it from
	// the JSON) so the card just hides the "Added X ago" line.
	if ts := parseUpdatedAt(picked.UpdatedAt); ts != nil {
		data.AddedAt = ts
	}
	if err := r.cache.Set(ctx, key, data, notTimeYetTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// parseUpdatedAt parses the player client's anime_list.updated_at string
// into a *time.Time. Returns nil for empty/zero/unparseable input so the
// caller can rely on omitempty to drop AddedAt from the JSON. Tries
// RFC3339 (with and without nanoseconds) — the formats Go's time.Time
// JSON marshaling and Postgres emit.
func parseUpdatedAt(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.IsZero() {
				return nil
			}
			return &t
		}
	}
	return nil
}
