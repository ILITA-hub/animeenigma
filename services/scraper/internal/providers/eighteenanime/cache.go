package eighteenanime

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Cache TTL constants. Mirrors the allanime/gogoanime cache pattern.
const (
	// mirrorsCacheTTL is the short cache for the parsed per-episode mirror list.
	// 18anime mirror links (mp4upload/turbovid embed URLs) rotate, so this is
	// deliberately short — just long enough to collapse the ListServers +
	// GetStream double-fetch within a single playback (finding L697).
	mirrorsCacheTTL = 5 * time.Minute
)

// cacheLayer wraps a libs/cache.Cache with eighteenanime-specific key shapes.
// A nil cacheLayer is a no-op (the provider runs cache-less when Deps.Cache is
// unset), so every method tolerates a nil receiver.
type cacheLayer struct {
	c cache.Cache
}

func newCacheLayer(c cache.Cache) *cacheLayer {
	if c == nil {
		return nil
	}
	return &cacheLayer{c: c}
}

func keyMirrors(episodeID string) string {
	return fmt.Sprintf("scraper:eighteenanime:mirrors:%s", episodeID)
}

// getMirrors returns the cached parsed mirror list for an episode, if present.
func (l *cacheLayer) getMirrors(ctx context.Context, episodeID string) ([]Mirror, bool) {
	if l == nil {
		return nil, false
	}
	var out []Mirror
	if err := l.c.Get(ctx, keyMirrors(episodeID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setMirrors(ctx context.Context, episodeID string, mirrors []Mirror) {
	if l == nil || len(mirrors) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyMirrors(episodeID), mirrors, mirrorsCacheTTL)
}
