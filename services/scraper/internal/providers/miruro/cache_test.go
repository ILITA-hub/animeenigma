package miruro

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Regression: sub and dub share a serverID (the inner-provider name, e.g.
// "kiwi"), so the stream cache key MUST include the category. Without it they
// collide on one entry and the first-fetched audio is served for both —
// "selected SUB, got DUB". See client.go GetStream.
func TestStreamCache_CategoryNamespaced(t *testing.T) {
	l := newCacheLayer(cache.Cache(newInMemoryCache()))
	ctx := context.Background()

	l.setStream(ctx, "147105", "ep1", "kiwi", "sub", &cachedStream{URL: "https://sub.example/s.m3u8", Type: "hls"})
	l.setStream(ctx, "147105", "ep1", "kiwi", "dub", &cachedStream{URL: "https://dub.example/d.m3u8", Type: "hls"})

	sub, ok := l.getStream(ctx, "147105", "ep1", "kiwi", "sub")
	if !ok || sub.URL != "https://sub.example/s.m3u8" {
		t.Fatalf("sub cache wrong (collision?): %+v ok=%v", sub, ok)
	}
	dub, ok := l.getStream(ctx, "147105", "ep1", "kiwi", "dub")
	if !ok || dub.URL != "https://dub.example/d.m3u8" {
		t.Fatalf("dub cache wrong (collision?): %+v ok=%v", dub, ok)
	}

	// Distinct keys per category.
	if keyStream("147105", "ep1", "kiwi", "sub") == keyStream("147105", "ep1", "kiwi", "dub") {
		t.Fatal("sub and dub produced the same cache key")
	}
}
