// Workstream hero-spotlight v1.0 Phase 3 — Plan 03-03 Task 3.
//
// TelegramNewsResolver implements spotlight.Resolver for the `telegram_news`
// card (HSB-BE-21). Reuses the EXISTING news:telegram Redis key (not a new
// spotlight:* prefix per HSB-NF-03) so the resolver and the existing
// /api/news handler share the same warm cache and TTL discipline (30
// minutes).
//
// Eligibility: empty posts → (nil, nil), no cache write.

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
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// telegramFetcher is the minimal surface this resolver depends on. The
// production *telegram.Client satisfies this implicitly.
type telegramFetcher interface {
	FetchNews(ctx context.Context) ([]telegram.NewsItem, error)
}

// telegramNewsKey is the EXISTING Redis key written by handler/news.go.
// Sharing the key means a fresh cache from /api/news warms this resolver
// and vice versa — HSB-NF-03 explicit exception to the spotlight:* prefix.
const telegramNewsKey = "news:telegram"

// telegramNewsTTL mirrors handler/news.go's newsTTL exactly.
const telegramNewsTTL = 30 * time.Minute

// TelegramNewsResolver implements spotlight.Resolver for `telegram_news`.
type TelegramNewsResolver struct {
	tg    telegramFetcher
	cache cache.Cache
	rng   *rand.Rand
	log   *logger.Logger
}

// NewTelegramNewsResolver constructs the resolver. rng may be nil — a
// time-seeded source is provided.
func NewTelegramNewsResolver(tg telegramFetcher, c cache.Cache, rng *rand.Rand, log *logger.Logger) *TelegramNewsResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &TelegramNewsResolver{tg: tg, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *TelegramNewsResolver) Type() string { return "telegram_news" }

// Resolve produces the telegram_news card. userID is ignored — every user
// sees the same telegram channel posts.
func (r *TelegramNewsResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	// --- Cache GET path -------------------------------------------------
	// The cached shape is []telegram.NewsItem (existing news handler), NOT
	// TelegramNewsData. We decode into the raw shape, then transform.
	var cached []telegram.NewsItem
	if err := r.cache.Get(ctx, telegramNewsKey, &cached); err == nil {
		return r.buildCardFromItems(cached), nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", telegramNewsKey, "error", err)
	}

	// --- Cache MISS path: fetch -----------------------------------------
	items, err := r.tg.FetchNews(ctx)
	if err != nil {
		return nil, fmt.Errorf("telegram_news: fetch: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Cache SET — write the raw []telegram.NewsItem shape so the news handler
	// can read it back unchanged. Encoding our TelegramNewsData here would
	// break /api/news (cache type mismatch).
	if data, mErr := json.Marshal(items); mErr == nil {
		// fakeCache.Set marshals again; the real RedisCache also re-marshals
		// via its JSON helper. Pass the slice directly so both paths agree.
		if err := r.cache.Set(ctx, telegramNewsKey, items, telegramNewsTTL); err != nil {
			r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", telegramNewsKey, "error", err)
		}
		_ = data
	}

	card := r.buildCardFromItems(items)
	if card == nil {
		// After AdaptiveSlice an empty list shouldn't happen here (we already
		// checked len(items) > 0), but defensively return ineligible.
		return nil, nil
	}
	return card, nil
}

// buildCardFromItems converts raw telegram items to a TelegramNewsData card,
// applying AdaptiveSlice. Returns nil for an empty input slice.
func (r *TelegramNewsResolver) buildCardFromItems(items []telegram.NewsItem) *spotlight.Card {
	if len(items) == 0 {
		return nil
	}
	posts := make([]spotlight.TelegramPost, 0, len(items))
	for _, it := range items {
		posts = append(posts, spotlight.TelegramPost{
			Excerpt: it.Text,
			Link:    it.Link,
			Date:    it.Date,
		})
	}
	picked := spotlight.AdaptiveSlice(posts, r.rng)
	if len(picked) == 0 {
		return nil
	}
	data := spotlight.TelegramNewsData{
		Posts: append([]spotlight.TelegramPost(nil), picked...),
	}
	return &spotlight.Card{Type: r.Type(), Data: data}
}
