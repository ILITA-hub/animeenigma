package nineanime

// cache.go — NineAnime per-key TTL strategies. Modeled after
// services/scraper/internal/providers/allanimeokru/cache.go +
// services/scraper/internal/providers/animepahe/malsync.go's positive +
// negative cache pattern.
//
// Key families (per 28-05-PLAN.md must_haves.truths):
//
//	scraper:nineanime:show:<cacheKey>            TTL 24h — slug lookup (positive)
//	scraper:nineanime:show:<cacheKey>:NEG        TTL 24h — slug lookup (negative)
//	scraper:nineanime:episodes:<slug>            TTL 6h  — episode listing
//	scraper:nineanime:servers:<slug>:<epURL>     TTL 1h  — single "1anime" entry
//	scraper:nineanime:stream:<slug>:<epURL>:<sv> TTL 5min — resolved MP4
//
// The negative cache for the show-ID lookup is a CONTEXT.md `<risks>` -
// driven optimization: 9anime.me.uk's brand-jack WP install lacks many
// anime (e.g. Frieren Season 1). Without negative caching, every miss
// re-hits the WP REST API at FindID time and wastes the per-host RPS
// budget. Pattern lifted from animepahe/malsync.go.
//
// Per CLAUDE.md "Don't cache video URLs longer than 1 hour" — streamTTLCap
// is 5min, matching allanime + animefever.

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Cache TTL constants.
const (
	// showIDCacheTTL is the 24h cache for title → 9anime slug resolution
	// (positive hit). Slugs are stable; brand-jack rebrands rarely.
	showIDCacheTTL = 24 * time.Hour

	// showIDNegCacheTTL is the 24h cache for negative WP-search misses.
	// Matches animepahe/malsync.go's negative-cache pattern. The window is
	// intentionally long so a miss does not re-hit the WP REST API on every
	// FindID call while the orchestrator failover chain is exhausted.
	showIDNegCacheTTL = 24 * time.Hour

	// episodesCacheTTL is the 6h cache for the assembled episode list.
	episodesCacheTTL = 6 * time.Hour

	// serversCacheTTL is the 1h cache for the per-episode server list.
	// 9anime publishes ONE server per episode (the my.1anime.site iframe);
	// the list is fixed, so the only reason to re-fetch is upstream rebrand.
	serversCacheTTL = 1 * time.Hour

	// streamTTLCap is the 5min cap on resolved MP4 URLs.
	streamTTLCap = 5 * time.Minute
)

// negSentinel is the value persisted under the negative-cache key. Any
// non-empty string works since we only consult presence; using a stable
// value lets the cache layer's serialization round-trip predictably.
const negSentinel = "NEG"

// cacheLayer wraps a libs/cache.Cache with nineanime-specific key shapes.
type cacheLayer struct {
	c cache.Cache
}

func newCacheLayer(c cache.Cache) *cacheLayer {
	return &cacheLayer{c: c}
}

// --- show ID (Title → 9anime slug) + negative cache ----------------------

func keyShowID(k string) string {
	return fmt.Sprintf("scraper:nineanime:show:%s", k)
}

func keyShowIDNeg(k string) string {
	return fmt.Sprintf("scraper:nineanime:show:%s:NEG", k)
}

// getShowID returns (slug, isNegative, ok). When ok=true, the slug is a
// real cached positive hit. When isNegative=true (and ok=true), a prior
// FindID call decided the WP REST API has no series match for the key;
// caller should short-circuit to ErrNotFound without an HTTP fetch.
func (l *cacheLayer) getShowID(ctx context.Context, k string) (slug string, isNegative bool, ok bool) {
	// Check negative cache first (cheaper to short-circuit).
	var neg string
	if err := l.c.Get(ctx, keyShowIDNeg(k), &neg); err == nil && neg != "" {
		return "", true, true
	}
	var out string
	if err := l.c.Get(ctx, keyShowID(k), &out); err == nil && out != "" {
		return out, false, true
	}
	return "", false, false
}

func (l *cacheLayer) setShowID(ctx context.Context, k, slug string) {
	_ = l.c.Set(ctx, keyShowID(k), slug, showIDCacheTTL)
}

// setShowIDNeg records a negative-cache hit for a key whose WP REST search
// returned no matching subtype:series result. Mirrors animepahe/malsync's
// per-MAL-ID negative cache.
func (l *cacheLayer) setShowIDNeg(ctx context.Context, k string) {
	if k == "" {
		return
	}
	_ = l.c.Set(ctx, keyShowIDNeg(k), negSentinel, showIDNegCacheTTL)
}

// --- episodes list --------------------------------------------------------

func keyEpisodes(slug string) string {
	return fmt.Sprintf("scraper:nineanime:episodes:%s", slug)
}

func (l *cacheLayer) getEpisodes(ctx context.Context, slug string) ([]episodeRef, bool) {
	var out []episodeRef
	if err := l.c.Get(ctx, keyEpisodes(slug), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setEpisodes(ctx context.Context, slug string, eps []episodeRef) {
	if len(eps) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyEpisodes(slug), eps, episodesCacheTTL)
}

// --- servers list (one episode) ------------------------------------------

func keyServers(slug, epURL string) string {
	return fmt.Sprintf("scraper:nineanime:servers:%s:%s", slug, epURL)
}

func (l *cacheLayer) getServers(ctx context.Context, slug, epURL string) ([]string, bool) {
	var out []string
	if err := l.c.Get(ctx, keyServers(slug, epURL), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, slug, epURL string, servers []string) {
	if len(servers) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(slug, epURL), servers, serversCacheTTL)
}

// --- stream URL (one server, one episode) --------------------------------

func keyStream(slug, epURL, server string) string {
	return fmt.Sprintf("scraper:nineanime:stream:%s:%s:%s", slug, epURL, server)
}

// cachedStream is what we persist in Redis for a resolved stream URL.
//
// Tracks/Intro/Outro are omitempty so the legacy my.1anime.site MP4 path
// (single source, no subs) round-trips byte-identically while the megaplay
// HLS path can persist its subtitle tracks + skip markers. Old cached
// entries written before these fields existed deserialize cleanly (the new
// fields stay nil).
type cachedStream struct {
	URL     string             `json:"url"`
	Type    string             `json:"type"`
	Quality string             `json:"quality"`
	Headers map[string]string  `json:"headers,omitempty"`
	Tracks  []domain.Track     `json:"tracks,omitempty"`
	Intro   *domain.TimeRange  `json:"intro,omitempty"`
	Outro   *domain.TimeRange  `json:"outro,omitempty"`
}

func (l *cacheLayer) getStream(ctx context.Context, slug, epURL, server string) (*cachedStream, bool) {
	var out cachedStream
	if err := l.c.Get(ctx, keyStream(slug, epURL, server), &out); err == nil && out.URL != "" {
		return &out, true
	}
	return nil, false
}

func (l *cacheLayer) setStream(ctx context.Context, slug, epURL, server string, s *cachedStream) {
	if s == nil || s.URL == "" {
		return
	}
	_ = l.c.Set(ctx, keyStream(slug, epURL, server), s, streamTTLCap)
}
