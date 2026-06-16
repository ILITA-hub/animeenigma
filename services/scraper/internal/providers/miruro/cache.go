package miruro

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Cache TTL constants — mirror allanime/cache.go for cross-provider
// consistency. The orchestrator/probe assumes "show ID never changes
// once resolved" (24h) and "stream URLs expire fast" (5m); diverging
// from those expectations confuses cross-provider debugging.
const (
	// showIDCacheTTL is the 24h cache for MAL → AniList ID resolution
	// via ARM (libs/idmapping). AniList IDs are stable.
	showIDCacheTTL = 24 * time.Hour

	// episodesCacheTTL is the 6h cache for the per-anime episode list.
	// Miruro re-renders this on upstream provider rotation; 6h matches
	// allanime/animepahe.
	episodesCacheTTL = 6 * time.Hour

	// serversCacheTTL is the 15min cache for the per-episode server
	// list (inner provider blocks: dune/kiwi/hop/bee).
	serversCacheTTL = 15 * time.Minute

	// streamTTLCap is the 5min cap on resolved stream URLs. Upstream
	// HLS edges (vault-*.uwucdn.top, etc.) sign URLs with short-lived
	// tokens; caching longer risks 403s.
	streamTTLCap = 5 * time.Minute
)

// cacheLayer wraps a libs/cache.Cache with miruro-specific key shapes.
type cacheLayer struct {
	c cache.Cache
}

func newCacheLayer(c cache.Cache) *cacheLayer {
	return &cacheLayer{c: c}
}

// --- show ID (Shikimori/MAL → AniList) ------------------------------------

func keyShowID(malID string) string {
	return fmt.Sprintf("scraper:miruro:show:%s", malID)
}

func (l *cacheLayer) getShowID(ctx context.Context, malID string) (string, bool) {
	var out string
	if err := l.c.Get(ctx, keyShowID(malID), &out); err == nil && out != "" {
		return out, true
	}
	return "", false
}

func (l *cacheLayer) setShowID(ctx context.Context, malID, aniListID string) {
	if aniListID == "" {
		return
	}
	_ = l.c.Set(ctx, keyShowID(malID), aniListID, showIDCacheTTL)
}

// --- episodes list (one anime, normalized across inner providers) ---------

func keyEpisodes(aniListID string) string {
	return fmt.Sprintf("scraper:miruro:episodes:%s", aniListID)
}

// cachedEpisode is the minimal shape we serialize per episode. We keep
// the upstream-opaque ID and the inner provider tag so ListServers can
// pivot back to the right `sources` query without re-fetching `episodes`.
type cachedEpisode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Filler   bool   `json:"filler"`
	Provider string `json:"provider"` // "dune" / "kiwi" / "hop" / "bee"
	Audio    string `json:"audio"`    // "sub" / "dub"
}

func (l *cacheLayer) getEpisodes(ctx context.Context, aniListID string) ([]cachedEpisode, bool) {
	var out []cachedEpisode
	if err := l.c.Get(ctx, keyEpisodes(aniListID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setEpisodes(ctx context.Context, aniListID string, eps []cachedEpisode) {
	if len(eps) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyEpisodes(aniListID), eps, episodesCacheTTL)
}

// --- servers list (one episode) -------------------------------------------

func keyServers(aniListID, episodeID string) string {
	return fmt.Sprintf("scraper:miruro:servers:%s:%s", aniListID, episodeID)
}

// cachedServer is what we persist per (anime, episode) for the server
// dropdown. Name is the inner provider ("dune"/"kiwi"/"hop"/"bee");
// Type is the audio category ("sub"/"dub").
type cachedServer struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	EpID  string `json:"ep_id"`
}

func (l *cacheLayer) getServers(ctx context.Context, aniListID, episodeID string) ([]cachedServer, bool) {
	var out []cachedServer
	if err := l.c.Get(ctx, keyServers(aniListID, episodeID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, aniListID, episodeID string, srvs []cachedServer) {
	if len(srvs) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(aniListID, episodeID), srvs, serversCacheTTL)
}

// --- stream URL (one server, one episode) ---------------------------------

func keyStream(aniListID, episodeID, server, category string) string {
	return fmt.Sprintf("scraper:miruro:stream:%s:%s:%s:%s", aniListID, episodeID, server, category)
}

// cachedStream is what we persist for a resolved stream URL.
type cachedStream struct {
	URL     string            `json:"url"`
	Type    string            `json:"type"`
	Quality string            `json:"quality"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (l *cacheLayer) getStream(ctx context.Context, aniListID, episodeID, server, category string) (*cachedStream, bool) {
	var out cachedStream
	if err := l.c.Get(ctx, keyStream(aniListID, episodeID, server, category), &out); err == nil && out.URL != "" {
		return &out, true
	}
	return nil, false
}

func (l *cacheLayer) setStream(ctx context.Context, aniListID, episodeID, server, category string, s *cachedStream) {
	if s == nil || s.URL == "" {
		return
	}
	_ = l.c.Set(ctx, keyStream(aniListID, episodeID, server, category), s, streamTTLCap)
}
