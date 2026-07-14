// Daily Fanfic Spotlight — Task 11.
//
// DailyFanficResolver implements spotlight.Resolver for the `daily_fanfic`
// card («Фанфик дня»). Global — every user sees the same fanfic on the same
// UTC day; userID is ignored.
//
// Mirrors featured.go's cache-get/cache-set discipline (DELIBERATE
// DIVERGENCE 1, see the cards package doc comment): manual `cache.Get` +
// `errors.Is(err, cache.ErrNotFound)` + `cache.Set`, with a no-cache-on-empty
// eligibility path so an upstream recovery (a fanfic becoming eligible
// later in the day) is retried on the next request rather than masked by a
// cached "nothing today" for the full 24h TTL.

package cards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// dailyFanficSource is the minimal surface this resolver depends on. The
// production *client.FanficClient satisfies this implicitly. Defined
// locally (not importing the concrete client type) so tests can substitute
// a handwritten fake — pattern: telegramFetcher / playerRecsFetcher.
type dailyFanficSource interface {
	FetchDaily(ctx context.Context) (*spotlight.DailyFanficData, error)
}

// DailyFanficResolver implements spotlight.Resolver for the `daily_fanfic` card.
type DailyFanficResolver struct {
	src   dailyFanficSource
	cache cache.Cache
	log   *logger.Logger
}

// NewDailyFanficResolver constructs the resolver. All deps are required;
// nil cache or src will panic on first Resolve.
func NewDailyFanficResolver(src dailyFanficSource, c cache.Cache, log *logger.Logger) *DailyFanficResolver {
	return &DailyFanficResolver{src: src, cache: c, log: log}
}

// Type returns the card discriminator string consumed by the frontend's
// TypeScript discriminated union.
func (r *DailyFanficResolver) Type() string { return "daily_fanfic" }

// Resolve returns the daily_fanfic card. userID is ignored — the pick is
// global (every user sees the same fanfic on the same UTC day).
//
// Eligibility: card is dropped (returns nil, nil) when the source has no
// daily fanfic (client 404 → (nil, nil)). The (nil, nil) path does NOT
// write to cache (Pitfall 5 from 01-RESEARCH.md).
func (r *DailyFanficResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:daily_fanfic:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.DailyFanficData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		// Hard-down Redis or decode error — log and fall through to the
		// fetch path. We never fail the user request because Redis is sick.
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: fetch from the fanfic service ------------------
	data, err := r.src.FetchDaily(ctx)
	if err != nil {
		return nil, fmt.Errorf("daily_fanfic: fetch: %w", err)
	}
	if data == nil {
		// Eligibility = false (no daily fanfic today). Do NOT cache empty.
		return nil, nil
	}

	// --- Cache SET (best-effort) ------------------------------------------
	if err := r.cache.Set(ctx, key, *data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: *data}, nil
}
