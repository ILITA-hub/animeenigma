package allanime

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

	// streamTTLCap is the 5min cap on resolved stream URLs.
	streamTTLCap = 5 * time.Minute
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

// --- servers list (one episode) ------------------------------------------

func keyServers(showID, ep string) string {
	return fmt.Sprintf("scraper:allanime:servers:%s:%s", showID, ep)
}

func (l *cacheLayer) getServers(ctx context.Context, showID, ep string) ([]sourceURL, bool) {
	var out []sourceURL
	if err := l.c.Get(ctx, keyServers(showID, ep), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, showID, ep string, src []sourceURL) {
	if len(src) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(showID, ep), src, serversCacheTTL)
}

// --- stream URL (one server, one episode) --------------------------------

func keyStream(showID, ep, server string) string {
	return fmt.Sprintf("scraper:allanime:stream:%s:%s:%s", showID, ep, server)
}

// cachedStream is what we persist in Redis for a resolved stream URL.
type cachedStream struct {
	URL       string            `json:"url"`
	Type      string            `json:"type"`
	Quality   string            `json:"quality"`
	Headers   map[string]string `json:"headers,omitempty"`
	Subtitles []cachedSubtitle  `json:"subtitles,omitempty"`
}

type cachedSubtitle struct {
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Label string `json:"label"`
}

func (l *cacheLayer) getStream(ctx context.Context, showID, ep, server string) (*cachedStream, bool) {
	var out cachedStream
	if err := l.c.Get(ctx, keyStream(showID, ep, server), &out); err == nil && out.URL != "" {
		return &out, true
	}
	return nil, false
}

func (l *cacheLayer) setStream(ctx context.Context, showID, ep, server string, s *cachedStream) {
	if s == nil || s.URL == "" {
		return
	}
	_ = l.c.Set(ctx, keyStream(showID, ep, server), s, streamTTLCap)
}
