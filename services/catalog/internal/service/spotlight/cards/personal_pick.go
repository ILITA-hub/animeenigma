// Workstream hero-spotlight v1.0 Phase 3 — Plan 03-03 Task 2.
//
// PersonalPickResolver implements spotlight.Resolver for the `personal_pick`
// card (HSB-BE-20). The resolver branches on userID:
//
//   - Anon (userID nil or empty): in-process catalog GetTrendingAnime call —
//     top 10 trending anime, then shuffled and reduced via AdaptiveSlice
//     (HSB-BE-30). Cache key spotlight:trending:<YYYY-MM-DD>, TTL 24h.
//     Card payload Source = "trending".
//
//   - Login (userID set): HTTP fan-out to player /api/users/recs via
//     PlayerClient.FetchUserRecs, forwarding the JWT from ctx. Player's
//     OptionalAuth returns personalized recs.upNext. Reduced via
//     AdaptiveSlice. Cache key spotlight:personal:<user_id>:<YYYY-MM-DD>,
//     TTL 24h. Card payload Source = "personal".
//
// If the login path is requested but the JWT is missing from ctx (defensive
// case — handler should always set it), the resolver falls back to the anon
// trending path so the card is still shown.

package cards

import (
	"context"
	"encoding/json"
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

// trendingFetcher is the minimal surface the anon path needs. The production
// *service.CatalogService satisfies this implicitly (see catalog.go:698).
type trendingFetcher interface {
	GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error)
}

// playerRecsFetcher is the minimal surface the login path needs. The
// production *client.PlayerClient satisfies this implicitly.
type playerRecsFetcher interface {
	FetchUserRecs(ctx context.Context, jwt string) ([]client.UserRec, error)
}

// personalPickTrendingPoolSize is the top-N trending pool the anon path
// pulls before AdaptiveSlice. 10 is a healthy buffer for the random-3
// rule and matches the design doc.
const personalPickTrendingPoolSize = 10

// personalPickTTL — 24h, mirroring anime_of_day. The day-keyed cache
// rotates at UTC midnight via DateKeyUTC.
const personalPickTTL = 24 * time.Hour

// PersonalPickResolver implements spotlight.Resolver for `personal_pick`.
type PersonalPickResolver struct {
	trending trendingFetcher
	recs     playerRecsFetcher
	cache    cache.Cache
	rng      *rand.Rand
	log      *logger.Logger
}

// NewPersonalPickResolver constructs the resolver. rng may be nil — a
// time-seeded source is provided.
func NewPersonalPickResolver(trending trendingFetcher, recs playerRecsFetcher, c cache.Cache, rng *rand.Rand, log *logger.Logger) *PersonalPickResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &PersonalPickResolver{trending: trending, recs: recs, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *PersonalPickResolver) Type() string { return "personal_pick" }

// Resolve produces the personal_pick card. userID == nil || *userID == "" →
// anon trending path. Non-empty userID → login fan-out path (with anon
// fallback if no JWT on ctx).
func (r *PersonalPickResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	isLogin := userID != nil && *userID != ""
	dateKey := spotlight.DateKeyUTC(time.Now())

	var key string
	if isLogin {
		key = "spotlight:personal:" + *userID + ":" + dateKey
	} else {
		key = "spotlight:trending:" + dateKey
	}

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.PersonalPickData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: branch on auth --------------------------------
	if isLogin {
		jwt, ok := JWTFromContext(ctx)
		if ok {
			return r.resolveLogin(ctx, key, jwt)
		}
		// Defensive fallback — handler should have set JWT, but if it didn't
		// we'd rather serve trending than a dark card. Fall through to anon.
		r.log.Warnw("spotlight.personal_pick.no_jwt_falling_back_to_anon", "user_id", *userID)
		// Re-key as anon so we don't pollute the login cache slot.
		key = "spotlight:trending:" + dateKey
		// Also check the anon cache before refetching.
		var anonCached spotlight.PersonalPickData
		if err := r.cache.Get(ctx, key, &anonCached); err == nil {
			return &spotlight.Card{Type: r.Type(), Data: anonCached}, nil
		} else if !errors.Is(err, cache.ErrNotFound) {
			r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
		}
	}
	return r.resolveAnon(ctx, key)
}

// resolveAnon executes the anon trending path: catalog.GetTrendingAnime →
// shuffle → AdaptiveSlice. cacheKey is "spotlight:trending:<YYYY-MM-DD>".
func (r *PersonalPickResolver) resolveAnon(ctx context.Context, cacheKey string) (*spotlight.Card, error) {
	animes, _, err := r.trending.GetTrendingAnime(ctx, 1, personalPickTrendingPoolSize)
	if err != nil {
		return nil, fmt.Errorf("personal_pick anon: %w", err)
	}
	if len(animes) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5).
		return nil, nil
	}

	items := make([]spotlight.PersonalPickItem, 0, len(animes))
	for _, a := range animes {
		if a == nil {
			continue
		}
		items = append(items, spotlight.PersonalPickItem{
			Anime:         *a,
			ReasonI18nKey: "spotlight.personalPick.reason.trending",
		})
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Shuffle the pool BEFORE AdaptiveSlice so the top 3 (or random 1 from
	// N==2) is varied across calls rather than always the top 3 by score.
	r.rng.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
	picked := spotlight.AdaptiveSlice(items, r.rng)
	if len(picked) == 0 {
		return nil, nil
	}

	data := spotlight.PersonalPickData{
		// Re-slice into a fresh backing array so the cached payload doesn't
		// share state with the pool we just shuffled.
		Items:  append([]spotlight.PersonalPickItem(nil), picked...),
		Source: "trending",
	}
	if err := r.cache.Set(ctx, cacheKey, data, personalPickTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", cacheKey, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// resolveLogin executes the login fan-out path: PlayerClient.FetchUserRecs →
// AdaptiveSlice. cacheKey is "spotlight:personal:<user_id>:<YYYY-MM-DD>".
func (r *PersonalPickResolver) resolveLogin(ctx context.Context, cacheKey, jwt string) (*spotlight.Card, error) {
	recs, err := r.recs.FetchUserRecs(ctx, jwt)
	if err != nil {
		return nil, fmt.Errorf("personal_pick login: %w", err)
	}
	if len(recs) == 0 {
		return nil, nil
	}

	// Take up to 10 raw recs, decode each, build items.
	maxRecs := personalPickTrendingPoolSize
	if len(recs) < maxRecs {
		maxRecs = len(recs)
	}
	items := make([]spotlight.PersonalPickItem, 0, maxRecs)
	for i := 0; i < maxRecs; i++ {
		var a domain.Anime
		if len(recs[i].Anime) == 0 {
			continue
		}
		if err := json.Unmarshal(recs[i].Anime, &a); err != nil {
			r.log.Warnw("spotlight.personal_pick.decode_failed", "index", i, "error", err)
			continue
		}
		items = append(items, spotlight.PersonalPickItem{
			Anime:         a,
			ReasonI18nKey: "spotlight.personalPick.reason.personal",
		})
	}
	if len(items) == 0 {
		return nil, nil
	}

	picked := spotlight.AdaptiveSlice(items, r.rng)
	if len(picked) == 0 {
		return nil, nil
	}

	data := spotlight.PersonalPickData{
		Items:  append([]spotlight.PersonalPickItem(nil), picked...),
		Source: "personal",
	}
	if err := r.cache.Set(ctx, cacheKey, data, personalPickTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", cacheKey, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
