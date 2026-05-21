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

// changelogFetcher is the subset of client.WebClient that
// LatestNewsResolver depends on. Defined locally so tests can swap in a
// handwritten fake (and so the cards package does not import its own
// sibling client subpackage at the type-system level).
type changelogFetcher interface {
	GetChangelog(ctx context.Context) ([]spotlight.ChangelogEntry, error)
}

// LatestNewsResolver implements spotlight.Resolver for the
// `latest_news` card. Fetches /changelog.json from the `web` container
// and returns the 3 newest changelog entries.
type LatestNewsResolver struct {
	web   changelogFetcher
	cache cache.Cache
	log   *logger.Logger
}

// NewLatestNewsResolver constructs the resolver.
func NewLatestNewsResolver(w changelogFetcher, c cache.Cache, log *logger.Logger) *LatestNewsResolver {
	return &LatestNewsResolver{web: w, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *LatestNewsResolver) Type() string { return "latest_news" }

// Resolve returns the latest_news card. userID is ignored — every user
// sees the same changelog. Eligibility: empty entries → (nil, nil),
// no cache write (Pitfall 5).
func (r *LatestNewsResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:changelog:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.LatestNewsData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: fetch via web client --------------------------
	entries, err := r.web.GetChangelog(ctx)
	if err != nil {
		return nil, fmt.Errorf("latest_news: fetch changelog: %w", err)
	}
	if len(entries) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5 + Pitfall 7).
		// This matters most on cold-start when `web` may not yet be ready —
		// if we cached empty for 24h, the card would stay dark all day.
		return nil, nil
	}
	data := spotlight.LatestNewsData{Entries: entries}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
