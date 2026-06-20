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
