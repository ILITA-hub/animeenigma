package cards

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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
// and applies the HSB-BE-30 1-2-3 adaptive layout rule via
// spotlight.AdaptiveSlice: N=0 → ineligible, N=1 → passthrough, N=2 →
// random pick of 1, N>=3 → top 3 (input order preserved).
type LatestNewsResolver struct {
	web   changelogFetcher
	cache cache.Cache
	rng   *rand.Rand
	log   *logger.Logger
}

// NewLatestNewsResolver constructs the resolver. rng may be nil — a
// time-seeded source is provided so production callers can omit it.
// Plan 03-04 (Phase 3 retrofit) added the rng parameter to drive the N=2
// random-pick branch of spotlight.AdaptiveSlice (HSB-BE-30).
func NewLatestNewsResolver(w changelogFetcher, c cache.Cache, rng *rand.Rand, log *logger.Logger) *LatestNewsResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &LatestNewsResolver{web: w, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *LatestNewsResolver) Type() string { return "latest_news" }

// Resolve returns the latest_news card. userID is ignored — every user
// sees the same changelog. Eligibility: empty entries → (nil, nil),
// no cache write (Pitfall 5). AdaptiveSlice is applied AFTER fetch and
// BEFORE the cache write so the already-narrowed payload is what gets
// cached — re-reads inside the TTL window return the same pick.
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

	// HSB-BE-30 adaptive layout — N=0 → nil, N=1 → passthrough, N=2 →
	// random pick of 1, N>=3 → top 3. Plan 03-04 retrofit (Phase 1
	// originally truncated to entries[:3] unconditionally).
	picked := spotlight.AdaptiveSlice(entries, r.rng)
	if len(picked) == 0 {
		return nil, nil
	}
	data := spotlight.LatestNewsData{Entries: picked}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
