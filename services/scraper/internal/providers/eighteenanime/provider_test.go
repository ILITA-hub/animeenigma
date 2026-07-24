package eighteenanime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// inMemoryCache is a tiny, race-safe cache.Cache impl backed by a map. Tests do
// not need TTL semantics; the ttl argument is ignored.
type inMemoryCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newInMemoryCache() *inMemoryCache { return &inMemoryCache{m: make(map[string][]byte)} }

func (c *inMemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	if !ok {
		return errors.New("miss")
	}
	return json.Unmarshal(v, dest)
}

func (c *inMemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = b
	return nil
}

func (c *inMemoryCache) Delete(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
	}
	return nil
}

func (c *inMemoryCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	if !ok {
		return errors.New("miss")
	}
	delete(c.m, key)
	return json.Unmarshal(v, dest)
}

func (c *inMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[key]
	return ok, nil
}

func (c *inMemoryCache) Invalidate(ctx context.Context, pattern string) error { return nil }

func (c *inMemoryCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := c.Get(ctx, key, dest); err == nil {
		return nil
	}
	val, err := fn()
	if err != nil {
		return err
	}
	if err := c.Set(ctx, key, val, ttl); err != nil {
		return err
	}
	return c.Get(ctx, key, dest)
}

func (c *inMemoryCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	_, ok := c.m[key]
	c.mu.Unlock()
	if ok {
		return false, nil
	}
	return true, c.Set(ctx, key, value, ttl)
}

// newFixtureServer serves the captured fixtures: GET /?s= search, the episode
// page, and an mp4upload-style embed page (for the GetStream failover path).
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	search, _ := os.ReadFile("testdata/search_results.html")
	episode, _ := os.ReadFile("testdata/episode_page.html")
	mp4embed, _ := os.ReadFile("testdata/embed_mp4upload.html")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("s") != "":
			_, _ = w.Write(search)
		case strings.Contains(r.URL.Path, "/embed-"):
			_, _ = w.Write(mp4embed)
		case strings.HasPrefix(r.URL.Path, "/hentai/"):
			_, _ = w.Write(episode)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestProvider(base string) *Provider {
	return New(Deps{BaseURL: base, SearchBase: base})
}

// hostRouteTransport sends every request to the fixture server, preserving
// path/query. It lets a test use a real embed host (mp4upload.com) in a Mirror
// link — which strict parsed-host matching now requires — while the actual
// fetch still lands on the local fixture's /embed- handler.
type hostRouteTransport struct{ target *url.URL }

func (t hostRouteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newRoutedProvider builds a test provider whose HTTP client routes embed
// fetches to the fixture regardless of the request host.
func newRoutedProvider(t *testing.T, srv *httptest.Server) *Provider {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	hc := domain.NewBaseHTTPClient(logger.Default(),
		domain.WithTransport(hostRouteTransport{target: target}),
		domain.WithProvider(providerName),
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	return New(Deps{BaseURL: srv.URL, SearchBase: srv.URL, HTTP: hc})
}

func TestProvider_Name(t *testing.T) {
	if New(Deps{}).Name() != "18anime" {
		t.Fatal("Name must be 18anime")
	}
}

func TestProvider_FindID(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// "JK to Inkou Kyoushi 4" matches two real series (…-4 and …-4-feat-…);
	// bestMatch breaks the tie by order. Either base is a valid resolution.
	id, err := p.FindID(context.Background(), domain.AnimeRef{Title: "JK to Inkou Kyoushi 4"})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if !strings.HasPrefix(id, "jk-to-inkou-kyoushi-4") {
		t.Fatalf("FindID base slug = %q", id)
	}

	if _, err := p.FindID(context.Background(), domain.AnimeRef{Title: "totally unrelated xyzzy"}); err == nil {
		t.Fatal("expected error for no match")
	}
}

func TestProvider_ListEpisodes(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// Exact-base grouping: the shorter series must not absorb the "-feat-..." one.
	eps, err := p.ListEpisodes(context.Background(), "jk-to-inkou-kyoushi-4")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 2 || eps[0].Number != 1 || eps[1].Number != 2 {
		t.Fatalf("episodes wrong: %+v", eps)
	}
	if !strings.Contains(eps[0].ID, "jk-to-inkou-kyoushi-4-episode-1") {
		t.Fatalf("episode ID wrong: %q", eps[0].ID)
	}
}

func TestProvider_ListServers(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	servers, err := p.ListServers(context.Background(), "base", "472-akiba-girls-episode-1")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	var hasMP4, hasTurbo bool
	for _, s := range servers {
		if s.ID == "mp4upload" {
			hasMP4 = true
		}
		if s.ID == "turbovid" {
			hasTurbo = true
		}
		if s.Type != domain.CategorySub {
			t.Fatalf("server %s type = %q, want sub", s.ID, s.Type)
		}
	}
	if !hasMP4 || !hasTurbo {
		t.Fatalf("expected both mp4upload + turbovid servers, got %+v", servers)
	}
	// mp4upload must come first (failover order).
	if servers[0].ID != "mp4upload" {
		t.Fatalf("expected mp4upload first, got %q", servers[0].ID)
	}
}

// countingRouteTransport reroutes every request to the fixture server
// (preserving path/query) and counts how many times the episode page
// (/hentai/...) is fetched, so a test can prove ListServers+GetStream share a
// single episode-page GET (finding L697).
type countingRouteTransport struct {
	target  *url.URL
	mu      sync.Mutex
	episode int // count of /hentai/ episode-page fetches
}

func (t *countingRouteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, "/hentai/") {
		t.mu.Lock()
		t.episode++
		t.mu.Unlock()
	}
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func (t *countingRouteTransport) episodeFetches() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.episode
}

// newCountingProvider builds a provider whose HTTP routes everything to the
// fixture and counts episode-page fetches. A shared Cache lets the memo persist.
func newCountingProvider(t *testing.T, srv *httptest.Server) (*Provider, *countingRouteTransport) {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	rt := &countingRouteTransport{target: target}
	hc := domain.NewBaseHTTPClient(logger.Default(),
		domain.WithTransport(rt),
		domain.WithProvider(providerName),
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	p := New(Deps{BaseURL: srv.URL, SearchBase: srv.URL, HTTP: hc, Cache: newInMemoryCache()})
	return p, rt
}

// TestProvider_SharesEpisodePageFetch proves ListServers then GetStream for the
// same episode fetch the episode page exactly ONCE — the parsed mirror list is
// memoized (finding L697). Before the cache, each call independently re-fetched
// EpisodeURL(episodeID) (count == 2).
func TestProvider_SharesEpisodePageFetch(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p, rt := newCountingProvider(t, srv)

	const episodeID = "472-akiba-girls-episode-1"
	if _, err := p.ListServers(context.Background(), "base", episodeID); err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if _, err := p.GetStream(context.Background(), "base", episodeID, "", domain.CategorySub); err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got := rt.episodeFetches(); got != 1 {
		t.Fatalf("episode page fetched %d times across ListServers+GetStream, want 1", got)
	}

	// A repeat GetStream within the TTL must add ZERO episode-page fetches.
	if _, err := p.GetStream(context.Background(), "base", episodeID, "", domain.CategorySub); err != nil {
		t.Fatalf("GetStream (repeat): %v", err)
	}
	if got := rt.episodeFetches(); got != 1 {
		t.Fatalf("episode page fetched %d times after a cached repeat GetStream, want 1", got)
	}
}

func TestProvider_resolveStream_Failover(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newRoutedProvider(t, srv)

	// Real embed host so strict parsed-host matching classifies it as mp4upload;
	// the routed client sends the actual fetch to the fixture's /embed- handler.
	mirrors := []Mirror{{Link: "https://www.mp4upload.com/embed-mp4upload-x.html", Quality: "FullHD"}}
	src, err := p.resolveStream(context.Background(), mirrors, "")
	if err != nil {
		t.Fatalf("resolveStream: %v", err)
	}
	if src.URL == "" || src.IsHLS {
		t.Fatalf("expected mp4 source, got %+v", src)
	}
	if src.Referer != "https://www.mp4upload.com/" {
		t.Fatalf("expected mp4upload referer, got %q", src.Referer)
	}
}

// tagCapturingTransport routes every request to the fixture (like
// hostRouteTransport) and records the scraper provider tag carried on each
// outbound request's context, so the test can prove the BaseHTTPClient pipeline
// (WithProvider egress tag) actually applies to 18anime's traffic.
type tagCapturingTransport struct {
	target *url.URL
	mu     sync.Mutex
	tags   []string
}

func (t *tagCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.tags = append(t.tags, domain.ProviderFromContext(req.Context()))
	t.mu.Unlock()
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// TestProvider_EgressProviderTag asserts that 18anime's upstream requests flow
// through domain.BaseHTTPClient and carry the WithProvider("18anime") egress
// tag (finding L690) — so the Activity-Register recorder pivots its egress rows
// by provider+host. Before the migration, New() ignored any *BaseHTTPClient and
// used a bare http.Client, leaving every request UNTAGGED (tag == "").
func TestProvider_EgressProviderTag(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	rt := &tagCapturingTransport{target: target}
	hc := domain.NewBaseHTTPClient(logger.Default(),
		domain.WithTransport(rt),
		domain.WithProvider(providerName),
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	p := New(Deps{BaseURL: srv.URL, SearchBase: srv.URL, HTTP: hc})

	// Exercise a real upstream fetch through fetch() (the BaseHTTPClient path).
	if _, err := p.ListServers(context.Background(), "base", "472-akiba-girls-episode-1"); err != nil {
		t.Fatalf("ListServers: %v", err)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.tags) == 0 {
		t.Fatal("no outbound requests captured")
	}
	for i, tag := range rt.tags {
		if tag != providerName {
			t.Fatalf("request %d carried provider tag %q, want %q (18anime not flowing through BaseHTTPClient)", i, tag, providerName)
		}
	}
}

func TestProvider_resolveStream_ServerPin(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// A real mp4upload mirror; pinning turbovid (absent) yields no supported mirror.
	mirrors := []Mirror{{Link: "https://www.mp4upload.com/embed-mp4upload-x.html"}} // mp4upload-only
	if _, err := p.resolveStream(context.Background(), mirrors, "turbovid"); err == nil {
		t.Fatal("expected error pinning absent server")
	}
}
