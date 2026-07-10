package gogoanime

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// newBrowserProvider builds a Provider with the browser-engine closures wired,
// reusing the shared test fakes.
func newBrowserProvider(t *testing.T, use bool, resolve BrowserResolveFunc) *Provider {
	t.Helper()
	log := newTestLogger(t)
	p, err := New(Deps{
		HTTP:           domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0)),
		Embeds:         domain.NewRegistry(),
		MalSync:        &fakeMalSync{mappings: map[string]string{}, misses: map[string]bool{}},
		Cache:          newFakeCache(),
		Log:            log,
		UseBrowser:     func() bool { return use },
		BrowserResolve: resolve,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func okStream() *domain.Stream {
	return &domain.Stream{Sources: []domain.Source{{URL: "http://stealth-scraper:3000/hls?sid=x&url=y", Type: "hls"}}}
}

// GetStream delegates the embed/server URL to the browser resolver when engine=browser.
func TestGetStream_BrowserEngine_Delegates(t *testing.T) {
	t.Parallel()
	var gotEmbed string
	p := newBrowserProvider(t, true, func(_ context.Context, embed string, _ domain.Category) (*domain.Stream, error) {
		gotEmbed = embed
		return okStream(), nil
	})
	embed := "https://gogoanime.me.uk/newplayer.php?id=foo?ep=42&type=hd-1&category=sub"
	st, err := p.GetStream(context.Background(), "pid", "eid", embed, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if gotEmbed != embed {
		t.Errorf("resolver got embed %q; want %q", gotEmbed, embed)
	}
	if len(st.Sources) != 1 {
		t.Errorf("sources = %d; want 1", len(st.Sources))
	}
}

// A resolver error propagates (and is classified into stage health, not panicked).
func TestGetStream_BrowserEngine_Error(t *testing.T) {
	t.Parallel()
	p := newBrowserProvider(t, true, func(context.Context, string, domain.Category) (*domain.Stream, error) {
		return nil, domain.WrapProviderDown(errors.New("boom"), "sidecar")
	})
	if _, err := p.GetStream(context.Background(), "p", "e", "https://megaplay.buzz/stream/s-2/1/sub", domain.CategorySub); err == nil {
		t.Fatal("want error, got nil")
	}
}

// The gated path skips the probe and resolves the first megaplay server.
func TestGetStreamWithGate_BrowserEngine_PicksMegaplayServer(t *testing.T) {
	t.Parallel()
	var gotEmbed string
	p := newBrowserProvider(t, true, func(_ context.Context, embed string, _ domain.Category) (*domain.Stream, error) {
		gotEmbed = embed
		return okStream(), nil
	})
	servers := []domain.Server{
		{ID: "https://filemoon.sx/e/abc", Name: "Moon", Type: domain.CategorySub},
		{ID: "https://gogoanime.me.uk/newplayer.php?id=x?ep=1", Name: "HD-1", Type: domain.CategorySub},
	}
	st, gated, err := p.GetStreamWithGate(context.Background(), "pid", "eid", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if gated {
		t.Error("gated = true; browser path must bypass the gate (false)")
	}
	if gotEmbed != servers[1].ID {
		t.Errorf("resolver got %q; want the megaplay server %q", gotEmbed, servers[1].ID)
	}
	if st == nil || len(st.Sources) != 1 {
		t.Error("want a stream with 1 source")
	}
}

// newGatedBrowserProvider builds a Provider directly (bypassing
// newBrowserProvider, which doesn't expose the underlying fakeCache or accept
// a SessionAlive closure) so the dead-sid gate tests can pre-seed the cache
// and assert on Delete calls.
func newGatedBrowserProvider(t *testing.T, resolve BrowserResolveFunc, sessionAlive func(context.Context, string) string) (*Provider, *fakeCache) {
	t.Helper()
	log := newTestLogger(t)
	fc := newFakeCache()
	p, err := New(Deps{
		HTTP:           domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0)),
		Embeds:         domain.NewRegistry(),
		MalSync:        &fakeMalSync{mappings: map[string]string{}, misses: map[string]bool{}},
		Cache:          fc,
		Log:            log,
		UseBrowser:     func() bool { return true },
		BrowserResolve: resolve,
		SessionAlive:   sessionAlive,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, fc
}

// TestGetStream_CachedDeadSidRefetches: a cache hit whose source URL embeds a
// stealth-scraper sid that the sidecar reports "gone" must be treated as a
// cache MISS — entry deleted, browser resolve re-run. Any other state (or a
// nil SessionAlive) serves the cache untouched.
func TestGetStream_CachedDeadSidRefetches(t *testing.T) {
	t.Parallel()

	const providerID, episodeID, serverID = "pid", "eid", "https://megaplay.buzz/stream/s-2/1/sub"
	cacheKey := streamCacheKey(providerID, episodeID, serverID)
	cachedStream := domain.Stream{
		Sources: []domain.Source{{URL: "http://stealth-scraper:3000/hls?sid=abc123def456abc123def456abc12345&url=y", Type: "hls"}},
	}
	plainCDNStream := domain.Stream{
		Sources: []domain.Source{{URL: "https://vault-99.owocdn.top/stream/uwu.m3u8", Type: "hls"}},
	}

	t.Run("gone -> refetches and deletes stale entry", func(t *testing.T) {
		t.Parallel()
		var resolveCalls int
		fresh := okStream()
		p, fc := newGatedBrowserProvider(t,
			func(context.Context, string, domain.Category) (*domain.Stream, error) {
				resolveCalls++
				return fresh, nil
			},
			func(context.Context, string) string { return "gone" },
		)
		if err := fc.Set(context.Background(), cacheKey, cachedStream, 0); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		st, err := p.GetStream(context.Background(), providerID, episodeID, serverID, domain.CategorySub)
		if err != nil {
			t.Fatalf("GetStream: %v", err)
		}
		if resolveCalls != 1 {
			t.Errorf("BrowserResolve calls = %d; want 1", resolveCalls)
		}
		if st.Sources[0].URL != fresh.Sources[0].URL {
			t.Errorf("got stale cached stream, want fresh resolve result")
		}
		deleted := fc.snapshotDeleted()
		if len(deleted) == 0 || deleted[0] != cacheKey {
			t.Errorf("deleted = %v; want first delete to be %q", deleted, cacheKey)
		}
	})

	t.Run("alive -> serves cache untouched", func(t *testing.T) {
		t.Parallel()
		var resolveCalls int
		p, fc := newGatedBrowserProvider(t,
			func(context.Context, string, domain.Category) (*domain.Stream, error) {
				resolveCalls++
				return okStream(), nil
			},
			func(context.Context, string) string { return "alive" },
		)
		if err := fc.Set(context.Background(), cacheKey, cachedStream, 0); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		st, err := p.GetStream(context.Background(), providerID, episodeID, serverID, domain.CategorySub)
		if err != nil {
			t.Fatalf("GetStream: %v", err)
		}
		if resolveCalls != 0 {
			t.Errorf("BrowserResolve calls = %d; want 0 (cache should be served)", resolveCalls)
		}
		if st.Sources[0].URL != cachedStream.Sources[0].URL {
			t.Errorf("got %q; want cached URL %q", st.Sources[0].URL, cachedStream.Sources[0].URL)
		}
		if len(fc.snapshotDeleted()) != 0 {
			t.Errorf("deleted = %v; want none", fc.snapshotDeleted())
		}
	})

	t.Run("nil SessionAlive -> gate disabled, serves cache untouched", func(t *testing.T) {
		t.Parallel()
		var resolveCalls int
		p, fc := newGatedBrowserProvider(t,
			func(context.Context, string, domain.Category) (*domain.Stream, error) {
				resolveCalls++
				return okStream(), nil
			},
			nil,
		)
		if err := fc.Set(context.Background(), cacheKey, cachedStream, 0); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		st, err := p.GetStream(context.Background(), providerID, episodeID, serverID, domain.CategorySub)
		if err != nil {
			t.Fatalf("GetStream: %v", err)
		}
		if resolveCalls != 0 {
			t.Errorf("BrowserResolve calls = %d; want 0 (gate disabled)", resolveCalls)
		}
		if st.Sources[0].URL != cachedStream.Sources[0].URL {
			t.Errorf("got %q; want cached URL %q", st.Sources[0].URL, cachedStream.Sources[0].URL)
		}
	})

	t.Run("no sid in cached URL -> SessionAlive not called, serves cache", func(t *testing.T) {
		t.Parallel()
		var resolveCalls, aliveCalls int
		p, fc := newGatedBrowserProvider(t,
			func(context.Context, string, domain.Category) (*domain.Stream, error) {
				resolveCalls++
				return okStream(), nil
			},
			func(context.Context, string) string {
				aliveCalls++
				return "gone"
			},
		)
		if err := fc.Set(context.Background(), cacheKey, plainCDNStream, 0); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		st, err := p.GetStream(context.Background(), providerID, episodeID, serverID, domain.CategorySub)
		if err != nil {
			t.Fatalf("GetStream: %v", err)
		}
		if aliveCalls != 0 {
			t.Errorf("SessionAlive calls = %d; want 0 (non-sidecar URL)", aliveCalls)
		}
		if resolveCalls != 0 {
			t.Errorf("BrowserResolve calls = %d; want 0", resolveCalls)
		}
		if st.Sources[0].URL != plainCDNStream.Sources[0].URL {
			t.Errorf("got %q; want cached URL %q", st.Sources[0].URL, plainCDNStream.Sources[0].URL)
		}
	})
}

func TestPickBrowserEmbed(t *testing.T) {
	t.Parallel()
	if got := pickBrowserEmbed(nil); got != "" {
		t.Errorf("empty servers -> %q; want \"\"", got)
	}
	servers := []domain.Server{
		{ID: "https://vidmoly.net/x"},
		{ID: "https://megaplay.buzz/stream/s-2/9/sub"},
	}
	if got := pickBrowserEmbed(servers); got != servers[1].ID {
		t.Errorf("pick = %q; want megaplay %q", got, servers[1].ID)
	}
	// No megaplay-family host -> first server.
	only := []domain.Server{{ID: "https://filemoon.sx/y"}}
	if got := pickBrowserEmbed(only); got != only[0].ID {
		t.Errorf("fallback pick = %q; want %q", got, only[0].ID)
	}
}
