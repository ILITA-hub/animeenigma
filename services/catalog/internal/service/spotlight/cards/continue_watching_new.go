// Workstream hero-spotlight v1.0 Phase 3 — Plan 03-03 Task 4 (Part B).
//
// ContinueWatchingNewResolver implements spotlight.Resolver for the
// `continue_watching_new` card (HSB-BE-25). Login-only — anon callers
// always get (nil, nil) BEFORE any data fetch (T-03-11).
//
// Resolves the user's anime_list filtered to status=watching, filters items
// where a NEW episode aired beyond the last viewing (EpisodesAired
// > LastWatchedEpisode + 1 — strict greater-than per spec), and picks the
// item with the most-recent UpdatedAt. Returns ineligible (nil, nil) when
// nothing matches.
//
// Cache key spotlight:continue_new:<user_id>, TTL 30 seconds.

package cards

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// continueWatchingNewKeyPrefix produces keys like "spotlight:continue_new:<uid>".
const continueWatchingNewKeyPrefix = "spotlight:continue_new:"

// continueWatchingNewTTL — 30s mirrors not_time_yet. Short enough that a
// newly-aired episode shows up promptly without hammering the player API.
const continueWatchingNewTTL = 30 * time.Second

// ContinueWatchingNewResolver implements spotlight.Resolver for the card.
type ContinueWatchingNewResolver struct {
	player listByStatusesFetcher
	cache  cache.Cache
	rng    *rand.Rand
	log    *logger.Logger
}

// NewContinueWatchingNewResolver constructs the resolver. rng may be nil —
// a time-seeded source is provided. The rng is currently unused (sort by
// UpdatedAt is deterministic) but kept on the struct for symmetry with the
// other resolvers and for future random-pick variants.
func NewContinueWatchingNewResolver(p listByStatusesFetcher, c cache.Cache, rng *rand.Rand, log *logger.Logger) *ContinueWatchingNewResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &ContinueWatchingNewResolver{player: p, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *ContinueWatchingNewResolver) Type() string { return "continue_watching_new" }

// Resolve produces the continue_watching_new card. Anon (userID nil/empty)
// returns (nil, nil) BEFORE any data fetch — login-only contract.
func (r *ContinueWatchingNewResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	if userID == nil || *userID == "" {
		return nil, nil
	}
	key := continueWatchingNewKeyPrefix + *userID

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.ContinueWatchingNewData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: fetch list ------------------------------------
	items, err := r.player.FetchListByStatuses(ctx, *userID, []string{"watching"})
	if err != nil {
		return nil, fmt.Errorf("continue_watching_new: list: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Filter to items where a NEW episode aired (strictly more than the
	// "next-after-last-watched" episode — per spec the rule is `>`, not `>=`,
	// which intentionally excludes "the next episode just aired" and includes
	// only "two or more new episodes since the user's last view").
	eligible := make([]client.InternalListItem, 0, len(items))
	for _, it := range items {
		if it.EpisodesAired > it.LastWatchedEpisode+1 {
			eligible = append(eligible, it)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}

	// Sort by UpdatedAt descending — ISO 8601 fixed-width strings sort
	// lexicographically the same as chronologically (no parse needed).
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].UpdatedAt > eligible[j].UpdatedAt
	})

	picked := eligible[0]
	anime := domain.Anime{
		ID:            picked.AnimeID,
		Name:          picked.Name,
		NameRU:        picked.NameRU,
		PosterURL:     picked.PosterURL,
		EpisodesAired: picked.EpisodesAired,
		EpisodesCount: picked.EpisodesCount,
	}
	data := spotlight.ContinueWatchingNewData{
		Anime:              anime,
		LastWatchedEpisode: picked.LastWatchedEpisode,
		NewEpisodeNumber:   picked.LastWatchedEpisode + 1,
	}

	if err := r.cache.Set(ctx, key, data, continueWatchingNewTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
