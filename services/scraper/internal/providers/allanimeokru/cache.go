package allanimeokru

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Cache TTL constants. Mirrors the gogoanime cache pattern.
const (
	// showIDCacheTTL is the 24h cache for MAL → AllAnime show ID resolution.
	// AllAnime IDs are stable; only the rare new-anime first-add changes them.
	showIDCacheTTL = 24 * time.Hour

	// episodesCacheTTL is the 6h cache for the assembled episode list.
	episodesCacheTTL = 6 * time.Hour

	// serversCacheTTL is the 15min cache for the per-episode server list.
	serversCacheTTL = 15 * time.Minute
)

// cacheLayer wraps a libs/cache.Cache with allanime-specific key shapes.
type cacheLayer struct {
	c cache.Cache
}

func newCacheLayer(c cache.Cache) *cacheLayer {
	return &cacheLayer{c: c}
}

// --- show ID (MAL → AllAnime _id) -----------------------------------------

func keyShowID(malID string) string {
	return fmt.Sprintf("scraper:allanime:show:%s", malID)
}

func (l *cacheLayer) getShowID(ctx context.Context, malID string) (string, bool) {
	var out string
	if err := l.c.Get(ctx, keyShowID(malID), &out); err == nil && out != "" {
		return out, true
	}
	return "", false
}

func (l *cacheLayer) setShowID(ctx context.Context, malID, showID string) {
	_ = l.c.Set(ctx, keyShowID(malID), showID, showIDCacheTTL)
}

// --- episodes list --------------------------------------------------------

func keyEpisodes(showID string) string {
	return fmt.Sprintf("scraper:allanime:episodes:%s", showID)
}

func (l *cacheLayer) getEpisodes(ctx context.Context, showID string) ([]string, bool) {
	var out []string
	if err := l.c.Get(ctx, keyEpisodes(showID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setEpisodes(ctx context.Context, showID string, eps []string) {
	if len(eps) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyEpisodes(showID), eps, episodesCacheTTL)
}

// --- available categories (which of sub/dub exist for a show) -------------
// Populated by ListEpisodes (free — it already fetches availableEpisodesDetail)
// and read by ListServers so it probes only categories that actually exist.

func keyCategories(showID string) string {
	return fmt.Sprintf("scraper:allanime:cats:%s", showID)
}

func (l *cacheLayer) getCategories(ctx context.Context, showID string) ([]string, bool) {
	var out []string
	if err := l.c.Get(ctx, keyCategories(showID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setCategories(ctx context.Context, showID string, cats []string) {
	if len(cats) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyCategories(showID), cats, episodesCacheTTL)
}

// --- servers list (one episode) ------------------------------------------

func keyServers(showID, ep, tt string) string {
	return fmt.Sprintf("scraper:allanime:servers:%s:%s:%s", showID, ep, tt)
}

func (l *cacheLayer) getServers(ctx context.Context, showID, ep, tt string) ([]sourceURL, bool) {
	var out []sourceURL
	if err := l.c.Get(ctx, keyServers(showID, ep, tt), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, showID, ep, tt string, src []sourceURL) {
	if len(src) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(showID, ep, tt), src, serversCacheTTL)
}
