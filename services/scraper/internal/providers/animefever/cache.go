package animefever

// cache.go — AnimeFever per-key TTL strategies. Modeled after
// services/scraper/internal/providers/allanime/cache.go.
//
// Key families (per 28-02-PLAN.md must_haves.truths):
//
//	scraper:animefever:show:<cacheKey>        TTL 24h — slug lookup
//	scraper:animefever:episodes:<slug>        TTL 6h  — episode listing
//	scraper:animefever:servers:<slug>:<eid>   TTL 1h  — watch-page server list
//	scraper:animefever:stream:<slug>:<eid>:<server>  TTL 5min — resolved stream
//	scraper:animefever:ctk:<slug>:<eid>       TTL 15min — CSRF token (Pitfall 2)
//
// Per CLAUDE.md "Don't cache video URLs longer than 1 hour" — streamTTLCap
// is 5min, matching allanime.

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Cache TTL constants. Mirrors the allanime cache pattern.
const (
	// showIDCacheTTL is the 24h cache for Title → AnimeFever slug resolution.
	// Slugs are stable; rare-anime first-add changes them.
	showIDCacheTTL = 24 * time.Hour

	// episodesCacheTTL is the 6h cache for the assembled episode list.
	episodesCacheTTL = 6 * time.Hour

	// serversCacheTTL is the 1h cache for the per-episode server list.
	// Longer than allanime's 15min because AnimeFever's advertised server set
	// is static (tserver only since AUTO-275; hserver is blocked) — what
	// changes is the ctk token (separate cache below). Cached entries are
	// filtered through supportedServers on read, so a pre-AUTO-275 entry
	// containing hserver cannot re-surface a blocked server.
	serversCacheTTL = 1 * time.Hour

	// streamTTLCap is the 5min cap on resolved stream URLs (signed m3u8
	// URLs expire fast). Matches allanime.
	streamTTLCap = 5 * time.Minute

	// ctkTTL is the watch-page `var ctk = '...'` token cache. The token
	// appears to be a CSRF-like anti-scrape mechanism and may rotate per
	// PHPSESSID lifetime. 15min is the conservative bound — if the AJAX
	// returns status:false, the GetStream path evicts and re-fetches.
	ctkTTL = 15 * time.Minute
)

// cacheLayer wraps a libs/cache.Cache with animefever-specific key shapes.
type cacheLayer struct {
	c cache.Cache
}

func newCacheLayer(c cache.Cache) *cacheLayer {
	return &cacheLayer{c: c}
}

// --- show ID (Title → AnimeFever slug) ------------------------------------

func keyShowID(k string) string {
	return fmt.Sprintf("scraper:animefever:show:%s", k)
}

func (l *cacheLayer) getShowID(ctx context.Context, k string) (string, bool) {
	var out string
	if err := l.c.Get(ctx, keyShowID(k), &out); err == nil && out != "" {
		return out, true
	}
	return "", false
}

func (l *cacheLayer) setShowID(ctx context.Context, k, slug string) {
	_ = l.c.Set(ctx, keyShowID(k), slug, showIDCacheTTL)
}

// --- episodes list --------------------------------------------------------

func keyEpisodes(slug string) string {
	return fmt.Sprintf("scraper:animefever:episodes:%s", slug)
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

func keyServers(slug, eid string) string {
	return fmt.Sprintf("scraper:animefever:servers:%s:%s", slug, eid)
}

func (l *cacheLayer) getServers(ctx context.Context, slug, eid string) ([]string, bool) {
	var out []string
	if err := l.c.Get(ctx, keyServers(slug, eid), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, slug, eid string, servers []string) {
	if len(servers) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(slug, eid), servers, serversCacheTTL)
}

// --- stream URL (one server, one episode) --------------------------------

func keyStream(slug, eid, server string) string {
	return fmt.Sprintf("scraper:animefever:stream:%s:%s:%s", slug, eid, server)
}

// cachedStream is what we persist in Redis for a resolved stream URL.
type cachedStream struct {
	URL     string            `json:"url"`
	Type    string            `json:"type"`
	Quality string            `json:"quality"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (l *cacheLayer) getStream(ctx context.Context, slug, eid, server string) (*cachedStream, bool) {
	var out cachedStream
	if err := l.c.Get(ctx, keyStream(slug, eid, server), &out); err == nil && out.URL != "" {
		return &out, true
	}
	return nil, false
}

func (l *cacheLayer) setStream(ctx context.Context, slug, eid, server string, s *cachedStream) {
	if s == nil || s.URL == "" {
		return
	}
	_ = l.c.Set(ctx, keyStream(slug, eid, server), s, streamTTLCap)
}

// --- ctk token (per slug+episode) ----------------------------------------

func keyCtk(slug, eid string) string {
	return fmt.Sprintf("scraper:animefever:ctk:%s:%s", slug, eid)
}

func (l *cacheLayer) getCtk(ctx context.Context, slug, eid string) (string, bool) {
	var out string
	if err := l.c.Get(ctx, keyCtk(slug, eid), &out); err == nil && out != "" {
		return out, true
	}
	return "", false
}

func (l *cacheLayer) setCtk(ctx context.Context, slug, eid, token string) {
	if token == "" {
		return
	}
	_ = l.c.Set(ctx, keyCtk(slug, eid), token, ctkTTL)
}

func (l *cacheLayer) deleteCtk(ctx context.Context, slug, eid string) {
	_ = l.c.Delete(ctx, keyCtk(slug, eid))
}
