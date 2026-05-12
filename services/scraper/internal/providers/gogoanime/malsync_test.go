package gogoanime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// TestMalSync_NegativeCacheForGogoanime verifies the forward-compat probe:
// the gogoanime malsync client calls api.malsync.moe with slug "Gogoanime"
// (matching malsync.moe's Sites-key capitalization convention) but the
// response (Plan 18-01 Task 3 golden malsync_no_gogo.json) lacks the key.
// The client returns ("", false, nil) and caches the miss for 24h.
// SCRAPER-9ANI-01.
func TestMalSync_NegativeCacheForGogoanime(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(goldenPath(t, "malsync_no_gogo.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/mal/anime/21" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	fc := newFakeCache()
	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))

	// First call: upstream HTTP fires once, miss returned, miss cached.
	id, ok, err := m.Lookup(context.Background(), "21", malSyncProviderSlug)
	if err != nil {
		t.Fatalf("first Lookup err = %v; want nil", err)
	}
	if ok || id != "" {
		t.Errorf("first Lookup = (%q,%v); want (\"\",false) — malsync_no_gogo.json has no Gogoanime key", id, ok)
	}
	if calls.Load() != 1 {
		t.Errorf("HTTP calls after first Lookup = %d; want 1", calls.Load())
	}
	// Negative cache key written?
	foundMiss := false
	for _, k := range fc.snapshotSetLog() {
		if k == "malsync:21:gogoanime:miss" {
			foundMiss = true
		}
	}
	if !foundMiss {
		t.Errorf("expected negative-cache write on malsync:21:gogoanime:miss; setLog=%v", fc.snapshotSetLog())
	}

	// Second Lookup with the same id: must hit the negative cache, NOT the
	// upstream. Verifies the forward-compat probe doesn't repeatedly drum
	// against malsync.moe for the steady-state miss.
	id2, ok2, err := m.Lookup(context.Background(), "21", malSyncProviderSlug)
	if err != nil {
		t.Fatalf("second Lookup err = %v", err)
	}
	if ok2 || id2 != "" {
		t.Errorf("second Lookup = (%q,%v); want (\"\",false)", id2, ok2)
	}
	if calls.Load() != 1 {
		t.Errorf("HTTP calls after second Lookup = %d; want 1 (negative cache should short-circuit)", calls.Load())
	}
}
