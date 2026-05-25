package animefever

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// Compile-time assertion: Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)

// --- in-memory cache (test double) ----------------------------------------

type inMemoryCache struct {
	m map[string][]byte
}

func newInMemoryCache() *inMemoryCache { return &inMemoryCache{m: make(map[string][]byte)} }

func (c *inMemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
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
	c.m[key] = b
	return nil
}
func (c *inMemoryCache) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		delete(c.m, k)
	}
	return nil
}
func (c *inMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.m[key]
	return ok, nil
}
func (c *inMemoryCache) Close() error                                       { return nil }
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
	if _, ok := c.m[key]; ok {
		return false, nil
	}
	return true, c.Set(ctx, key, value, ttl)
}

// --- fake embed extractor (test double) -----------------------------------

type fakeExtractor struct {
	name         string
	matchHost    string
	returnStream *domain.Stream
	returnErr    error
	lastURL      string
	lastHeaders  http.Header
}

func (e *fakeExtractor) Name() string { return e.name }
func (e *fakeExtractor) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, e.matchHost)
}
func (e *fakeExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	e.lastURL = embedURL
	e.lastHeaders = headers
	if e.returnErr != nil {
		return nil, e.returnErr
	}
	return e.returnStream, nil
}

// readFixture loads a testdata file by name.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// newTestProvider builds a Provider talking to httpSrv. Compresses retry
// backoff. Optionally accepts a fake extractor; otherwise registers a
// default vidstream.vip stub.
func newTestProvider(t *testing.T, httpSrv *httptest.Server, ext *fakeExtractor) (*Provider, *fakeExtractor) {
	t.Helper()
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log,
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	registry := domain.NewRegistry()
	if ext == nil {
		ext = &fakeExtractor{
			name:      "vidstream_vip",
			matchHost: "am.vidstream.vip",
			returnStream: &domain.Stream{
				Sources: []domain.Source{
					{URL: "https://static-cdn-ca1.mofl.pro/master.m3u8", Type: "hls", Quality: "auto"},
				},
				Headers: map[string]string{"Referer": "https://am.vidstream.vip/"},
			},
		}
	}
	registry.Register(ext)
	c := cache.Cache(newInMemoryCache())
	p, err := New(Deps{
		BaseURL: httpSrv.URL,
		HTTP:    base,
		Embeds:  registry,
		Cache:   c,
		Log:     log,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, ext
}

// --- New() validation -----------------------------------------------------

func TestNew_RequiresHTTP(t *testing.T) {
	_, err := New(Deps{Embeds: domain.NewRegistry(), Cache: newInMemoryCache()})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected HTTP required error, got %v", err)
	}
}

func TestNew_RequiresEmbeds(t *testing.T) {
	log := logger.Default()
	_, err := New(Deps{HTTP: domain.NewBaseHTTPClient(log), Cache: newInMemoryCache()})
	if err == nil || !strings.Contains(err.Error(), "Embeds") {
		t.Fatalf("expected Embeds required error, got %v", err)
	}
}

func TestNew_RequiresCache(t *testing.T) {
	log := logger.Default()
	_, err := New(Deps{HTTP: domain.NewBaseHTTPClient(log), Embeds: domain.NewRegistry()})
	if err == nil || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("expected Cache required error, got %v", err)
	}
}

func TestNew_Name(t *testing.T) {
	log := logger.Default()
	p, err := New(Deps{
		HTTP:   domain.NewBaseHTTPClient(log),
		Embeds: domain.NewRegistry(),
		Cache:  newInMemoryCache(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "animefever" {
		t.Fatalf("Name() = %q; want animefever", p.Name())
	}
}

// --- FindID --------------------------------------------------------------

func TestFindID_Frieren(t *testing.T) {
	searchHTML := readFixture(t, "search_frieren.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pitfall 1: search path is /search/<term>, NOT /search?keyword=.
		if !strings.HasPrefix(r.URL.Path, "/search/") {
			t.Errorf("expected /search/<term> path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(searchHTML)
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	id, err := p.FindID(context.Background(), domain.AnimeRef{
		ShikimoriID: "52991",
		Title:       "Frieren - Beyond Journey's End",
	})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if id != "frieren-beyond-journeys-end.14401" {
		t.Fatalf("FindID = %q; want frieren-beyond-journeys-end.14401", id)
	}

	// markStage on success
	snap := p.HealthCheck(context.Background())
	if !snap.Stages[health.StageSearch].Up {
		t.Errorf("StageSearch.Up = false after success; want true")
	}
	if snap.Stages[health.StageSearch].LastErr != "" {
		t.Errorf("StageSearch.LastErr = %q; want empty", snap.Stages[health.StageSearch].LastErr)
	}
}

func TestFindID_EmptyTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream should NOT be hit on empty title; got request to %s", r.URL.Path)
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	_, err := p.FindID(context.Background(), domain.AnimeRef{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty title, got %v", err)
	}
}

func TestFindID_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return search HTML that has no card-block elements matching the query.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body><div class="card-block"><a href="/info/totally-unrelated.1" title="Totally Unrelated"><h3>Totally Unrelated</h3></a></div></body></html>`))
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	_, err := p.FindID(context.Background(), domain.AnimeRef{
		Title: "Frieren Beyond Journey's End",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for low-score match, got %v", err)
	}
}

func TestFindID_TransportFailure_MarksStageDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Frieren"})
	if err == nil {
		t.Fatal("expected error on 5xx, got nil")
	}
	snap := p.HealthCheck(context.Background())
	if snap.Stages[health.StageSearch].Up {
		t.Errorf("StageSearch.Up = true after failure; want false")
	}
	if snap.Stages[health.StageSearch].LastErr == "" {
		t.Errorf("StageSearch.LastErr = empty after failure; want non-empty")
	}
}

// --- ListEpisodes --------------------------------------------------------

func TestListEpisodes_Frieren(t *testing.T) {
	infoHTML := readFixture(t, "info_frieren.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/info/") {
			t.Errorf("expected /info/<slug>, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(infoHTML)
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	eps, err := p.ListEpisodes(context.Background(), "frieren-beyond-journeys-end.14401")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) < 28 {
		t.Fatalf("ListEpisodes returned %d episodes; want ≥28", len(eps))
	}
	// Sorted ascending
	for i := 1; i < len(eps); i++ {
		if eps[i].Number < eps[i-1].Number {
			t.Errorf("episodes not sorted ascending at index %d: %d < %d", i, eps[i].Number, eps[i-1].Number)
		}
	}
	// ID format is <slug>:<eid>
	if !strings.Contains(eps[0].ID, ":") {
		t.Errorf("Episode.ID = %q; want format <slug>:<eid>", eps[0].ID)
	}
}

// --- ListServers --------------------------------------------------------

func TestListServers_BothTServerAndHServer(t *testing.T) {
	watchHTML := readFixture(t, "watch_ep28.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/watch/") {
			t.Errorf("expected /watch/<slug>, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(watchHTML)
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	servers, err := p.ListServers(
		context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028",
	)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("ListServers returned %d servers; want exactly 2 (tserver, hserver)", len(servers))
	}
	// Order matters per Pitfall 3: tserver first
	if servers[0].ID != "tserver" {
		t.Errorf("servers[0].ID = %q; want tserver", servers[0].ID)
	}
	if servers[1].ID != "hserver" {
		t.Errorf("servers[1].ID = %q; want hserver", servers[1].ID)
	}
	for _, s := range servers {
		if s.Type != domain.CategorySub {
			t.Errorf("server %q Type = %v; want CategorySub", s.ID, s.Type)
		}
	}
}

// --- GetStream --------------------------------------------------------

func TestGetStream_DelegatesToVidstreamVipExtractor(t *testing.T) {
	watchHTML := readFixture(t, "watch_ep28.html")
	ajaxJSON := readFixture(t, "ajax_load_ep28.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ajax/anime/load_episodes_v2":
			// Verify form-encoded body has episode_id + ctk
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			if r.PostForm.Get("episode_id") == "" {
				t.Errorf("missing episode_id in form")
			}
			if r.PostForm.Get("ctk") == "" {
				t.Errorf("missing ctk in form")
			}
			// Verify s=<server> query param + headers
			if r.URL.Query().Get("s") == "" {
				t.Errorf("missing s=<server> query param")
			}
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Errorf("missing X-Requested-With header")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(ajaxJSON)
		case strings.HasPrefix(r.URL.Path, "/watch/"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(watchHTML)
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	fakeExt := &fakeExtractor{
		name:      "vidstream_vip",
		matchHost: "am.vidstream.vip",
		returnStream: &domain.Stream{
			Sources: []domain.Source{
				{URL: "https://static-cdn-ca1.mofl.pro/master.m3u8", Type: "hls", Quality: "auto"},
			},
			Headers: map[string]string{"Referer": "https://am.vidstream.vip/"},
		},
	}
	p, _ := newTestProvider(t, srv, fakeExt)
	stream, err := p.GetStream(
		context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028",
		"tserver",
		domain.CategorySub,
	)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(stream.Sources) != 1 {
		t.Fatalf("Sources len = %d; want 1", len(stream.Sources))
	}
	if stream.Sources[0].URL != "https://static-cdn-ca1.mofl.pro/master.m3u8" {
		t.Errorf("Sources[0].URL = %q; want the extractor's URL unchanged", stream.Sources[0].URL)
	}
	if !strings.Contains(fakeExt.lastURL, "am.vidstream.vip") {
		t.Errorf("extractor routed URL = %q; want a URL on am.vidstream.vip", fakeExt.lastURL)
	}
}

func TestGetStream_NoMatchingExtractor_ReturnsExtractFailed(t *testing.T) {
	watchHTML := readFixture(t, "watch_ep28.html")
	// AJAX returns an iframe pointing at a host NO extractor matches.
	ajaxBody := []byte(`{"status":true,"value":"<iframe src=\"https://unknown-embed.example/embed/x?lt=ts\"></iframe>","embed":true}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ajax/anime/load_episodes_v2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(ajaxBody)
		case strings.HasPrefix(r.URL.Path, "/watch/"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(watchHTML)
		}
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil) // default extractor only matches am.vidstream.vip
	_, err := p.GetStream(
		context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028",
		"tserver",
		domain.CategorySub,
	)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed when no extractor matches, got %v", err)
	}
}

func TestGetStream_AjaxStatusFalse_ReturnsExtractFailed(t *testing.T) {
	watchHTML := readFixture(t, "watch_ep28.html")
	ajaxFail := []byte(`{"status":false}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ajax/anime/load_episodes_v2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(ajaxFail)
		case strings.HasPrefix(r.URL.Path, "/watch/"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(watchHTML)
		}
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	_, err := p.GetStream(
		context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028",
		"tserver",
		domain.CategorySub,
	)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for status:false AJAX, got %v", err)
	}
}

// TestFindID_MultiTitleResolvesRomajiMainSeries — ISS-017. AnimeFever lists the
// main series under its ROMAJI title and a recap compilation under the ENGLISH
// title. With only the English query the compilation wins (its title scores
// high); the romaji alt-title must flip the match to the main series.
func TestFindID_MultiTitleResolvesRomajiMainSeries(t *testing.T) {
	const html = `<!doctype html><html><body>` +
		`<div class="card-block" title="Attack on Titan Chronicle"><a href="/info/attack-on-titan-chronicle.60449" title="Attack on Titan Chronicle"><h3>Attack on Titan Chronicle</h3></a></div>` +
		`<div class="card-block" title="Shingeki no Kyojin"><a href="/info/shingeki-no-kyojin-sub.50313" title="Shingeki no Kyojin"><h3>Shingeki no Kyojin</h3></a></div>` +
		`</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, html)
	}))
	defer srv.Close()

	// Control (fresh provider → fresh show-id cache): English-only query picks
	// the compilation — this is the bug the alt-title fixes.
	pEN, _ := newTestProvider(t, srv, nil)
	idEN, err := pEN.FindID(context.Background(), domain.AnimeRef{Title: "Attack on Titan"})
	if err != nil {
		t.Fatalf("FindID (en-only): %v", err)
	}
	if idEN != "attack-on-titan-chronicle.60449" {
		t.Fatalf("en-only FindID = %q; want the compilation (demonstrates the matching gap)", idEN)
	}

	// With the romaji alt-title, the romaji-listed main series wins.
	pMulti, _ := newTestProvider(t, srv, nil)
	id, err := pMulti.FindID(context.Background(), domain.AnimeRef{
		Title:     "Attack on Titan",
		AltTitles: []string{"Shingeki no Kyojin"},
	})
	if err != nil {
		t.Fatalf("FindID (multi-title): %v", err)
	}
	if id != "shingeki-no-kyojin-sub.50313" {
		t.Fatalf("multi-title FindID = %q; want shingeki-no-kyojin-sub.50313 (romaji main series)", id)
	}
}

// TestGetStream_FreshCtkStatusFalse_NoEmbedNoRetry — ISS-017. A status:false
// with a FRESHLY-scraped ctk means "no embed for this episode/server", NOT a
// stale token. The provider must NOT waste a retry (ajax hit exactly once),
// must NOT report errStaleCtk, and must bump the no_embed metric.
func TestGetStream_FreshCtkStatusFalse_NoEmbedNoRetry(t *testing.T) {
	before := testutil.ToFloat64(metrics.ParserZeroMatchTotal.WithLabelValues("animefever", "no_embed"))
	watchHTML := readFixture(t, "watch_ep28.html")
	var ajaxCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ajax/anime/load_episodes_v2":
			atomic.AddInt32(&ajaxCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":false,"value":"","embed":false}`)
		case strings.HasPrefix(r.URL.Path, "/watch/"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(watchHTML)
		}
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	_, err := p.GetStream(context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028", "tserver", domain.CategorySub)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("err = %v; want ErrExtractFailed", err)
	}
	if errors.Is(err, errStaleCtk) {
		t.Errorf("err must NOT be errStaleCtk for a fresh-ctk no-embed; got %v", err)
	}
	if n := atomic.LoadInt32(&ajaxCalls); n != 1 {
		t.Errorf("ajax calls = %d; want 1 (no wasteful retry on a fresh ctk)", n)
	}
	if d := testutil.ToFloat64(metrics.ParserZeroMatchTotal.WithLabelValues("animefever", "no_embed")) - before; d != 1 {
		t.Errorf("no_embed metric delta = %v; want 1", d)
	}
}

// TestGetStream_CachedCtkStatusFalse_RetriesOnce — ISS-017. A status:false with
// a CACHED ctk could be a genuinely-stale token, so the provider evicts it and
// retries ONCE with a freshly-scraped token (ajax hit twice).
func TestGetStream_CachedCtkStatusFalse_RetriesOnce(t *testing.T) {
	watchHTML := readFixture(t, "watch_ep28.html")
	var ajaxCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ajax/anime/load_episodes_v2":
			atomic.AddInt32(&ajaxCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":false,"value":"","embed":false}`)
		case strings.HasPrefix(r.URL.Path, "/watch/"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(watchHTML)
		}
	}))
	defer srv.Close()

	p, _ := newTestProvider(t, srv, nil)
	// Pre-seed a CACHED ctk so the first attempt takes the stale-suspect path.
	p.cache.setCtk(context.Background(), "frieren-beyond-journeys-end.14401", "1028", "cafebabecafebabe")
	_, err := p.GetStream(context.Background(),
		"frieren-beyond-journeys-end.14401",
		"frieren-beyond-journeys-end.14401:1028", "tserver", domain.CategorySub)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("err = %v; want ErrExtractFailed", err)
	}
	if n := atomic.LoadInt32(&ajaxCalls); n != 2 {
		t.Errorf("ajax calls = %d; want 2 (cached-ctk stale → evict + retry once)", n)
	}
}

// --- Defensive: no forbidden imports -------------------------------------

// (Forbidden import enforcement lives in the existing
// services/scraper/internal/golint/forbidden_deps_test.go gate; we don't
// duplicate it here.)
