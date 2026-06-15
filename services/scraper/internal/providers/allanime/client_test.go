package allanime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Compile-time interface assertion. Failing this is a BUILD ERROR, the
// strongest possible test that Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)

// --- helpers --------------------------------------------------------------

// inMemoryCache is a tiny cache.Cache impl backed by a map. Tests do not
// need TTL semantics; we ignore the ttl argument entirely.
type inMemoryCache struct {
	m map[string][]byte
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{m: make(map[string][]byte)}
}

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

func (c *inMemoryCache) Close() error                                         { return nil }
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

// newTestProvider builds a Provider talking to httpSrv. Compresses retry
// backoff so unit tests don't sit waiting.
func newTestProvider(t *testing.T, httpSrv *httptest.Server) *Provider {
	t.Helper()
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log,
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	c := cache.Cache(newInMemoryCache())
	p, err := New(Deps{
		BaseURL: httpSrv.URL,
		HTTP:    base,
		Cache:   c,
		Log:     log,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// --- New() validation -----------------------------------------------------

func TestNew_RequiresHTTP(t *testing.T) {
	_, err := New(Deps{Cache: newInMemoryCache()})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected HTTP required error, got %v", err)
	}
}

func TestNew_RequiresCache(t *testing.T) {
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log)
	_, err := New(Deps{HTTP: base})
	if err == nil || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("expected Cache required error, got %v", err)
	}
}

func TestNew_Name(t *testing.T) {
	log := logger.Default()
	p, err := New(Deps{HTTP: domain.NewBaseHTTPClient(log), Cache: newInMemoryCache()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "allanime" {
		t.Fatalf("expected Name() = allanime, got %s", p.Name())
	}
}

// --- FindID --------------------------------------------------------------

func TestFindID_Frieren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "variables=") {
			t.Errorf("expected variables query param")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"shows":{"edges":[{"_id":"frieren_show_id","name":"Frieren","englishName":"Frieren: Beyond Journey's End","nativeName":"葬送のフリーレン","thumbnail":"x","availableEpisodes":{"sub":28}}]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	id, err := p.FindID(context.Background(), domain.AnimeRef{
		ShikimoriID: "52991",
		Title:       "Frieren",
	})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if id != "frieren_show_id" {
		t.Fatalf("expected show ID frieren_show_id, got %q", id)
	}
}

func TestFindID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"shows":{"edges":[]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{
		Title: "Nonexistent Anime XYZ",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFindID_EmptyTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should NOT be hit on empty title")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty title, got %v", err)
	}
}

// --- ListEpisodes --------------------------------------------------------

func TestListEpisodes_Frieren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"show":{"_id":"frieren_show_id","availableEpisodesDetail":{"sub":["1","2","3"],"dub":[],"raw":[]}}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	eps, err := p.ListEpisodes(context.Background(), "frieren_show_id")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(eps))
	}
	if eps[0].Number != 1 || eps[0].ID != "frieren_show_id:1" {
		t.Fatalf("ep[0] = %+v", eps[0])
	}
}

func TestListEpisodes_RealEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"show":{"_id":"x","availableEpisodesDetail":{"sub":[]}}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	eps, err := p.ListEpisodes(context.Background(), "x")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 episodes (real empty), got %d", len(eps))
	}
}

// --- ListServers + GetStream --------------------------------------------

func TestListServers_Frieren_Ep1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.example.com/frieren-ep1.m3u8","sourceName":"Default","type":"iframe","priority":9,"fileExtenstion":"m3u8"},{"sourceUrl":"https://cdn.example.com/frieren-ep1.mp4","sourceName":"S-mp4","type":"player","priority":7,"fileExtenstion":"mp4"}]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	servers, err := p.ListServers(context.Background(), "", "frieren_show_id:1")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Name != "Default" || servers[0].Type != domain.CategorySub {
		t.Fatalf("server[0] = %+v", servers[0])
	}
}

// allanimeTestServer serves both the GraphQL API (any non-.m3u8/.embed path)
// and the source URLs the provider then probes: a *.m3u8 path returns an HLS
// manifest (→ Stream), an *.embed path returns an HTML page (→ Embed). The
// sourceUrls in the API JSON point back at this same server so GetStream's
// content-probe sees deterministic, in-process content (no real network).
func allanimeTestServer(t *testing.T, sourcesJSONf func(base string) string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n"))
		case strings.HasSuffix(r.URL.Path, ".embed"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><html><body>embed player</body></html>"))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(sourcesJSONf(srv.URL)))
		}
	}))
	return srv
}

func TestGetStream_HappyPath(t *testing.T) {
	srv := allanimeTestServer(t, func(base string) string {
		return `{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"` + base + `/s.m3u8","sourceName":"Default","type":"iframe","priority":9,"fileExtenstion":"m3u8"}]}}}`
	})
	defer srv.Close()

	p := newTestProvider(t, srv)
	stream, err := p.GetStream(context.Background(), "", "frieren_show_id:1", "Default", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(stream.Sources) == 0 || stream.Sources[0].URL == "" {
		t.Fatalf("expected non-empty source URL, got %+v", stream.Sources)
	}
	if stream.Headers["Referer"] != "https://allmanga.to" {
		t.Fatalf("expected Referer=https://allmanga.to, got %q", stream.Headers["Referer"])
	}
}

// TestGetStream_AllEmbeds_FailsOver — when every source is an HTML embed page,
// GetStream returns ErrExtractFailed so the orchestrator fails over.
func TestGetStream_AllEmbeds_FailsOver(t *testing.T) {
	srv := allanimeTestServer(t, func(base string) string {
		return `{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"` + base + `/ok.embed","sourceName":"Ok","priority":9}]}}}`
	})
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetStream(context.Background(), "", "frieren_show_id:1", "Ok", domain.CategorySub)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for all-embed sources, got %v", err)
	}
}

// TestGetStream_SkipsEmbedPicksStream — even when the caller pins an embed
// server, GetStream transparently falls back to the playable source (dynamic
// classification, no host list).
func TestGetStream_SkipsEmbedPicksStream(t *testing.T) {
	srv := allanimeTestServer(t, func(base string) string {
		return `{"data":{"episode":{"episodeString":"1","sourceUrls":[` +
			`{"sourceUrl":"` + base + `/ok.embed","sourceName":"Ok","priority":9},` +
			`{"sourceUrl":"` + base + `/good.m3u8","sourceName":"Default","priority":5,"fileExtenstion":"m3u8"}]}}}`
	})
	defer srv.Close()

	p := newTestProvider(t, srv)
	// Pin the embed server "Ok"; expect fallback to the playable "Default".
	stream, err := p.GetStream(context.Background(), "", "frieren_show_id:1", "Ok", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(stream.Sources) == 0 || !strings.HasSuffix(stream.Sources[0].URL, "/good.m3u8") {
		t.Fatalf("expected fallback to /good.m3u8 stream, got %+v", stream.Sources)
	}
}

// TestListServers_SubAndDub verifies that ListServers returns both sub-tagged
// and dub-tagged servers when the categories cache records both "sub" and "dub"
// for the show.
func TestListServers_SubAndDub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "%22dub%22") {
			_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/dub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/sub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	p.cache.setCategories(context.Background(), "SHOW123", []string{"sub", "dub"})

	servers, err := p.ListServers(context.Background(), "", "SHOW123:1")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	var sawSub, sawDub bool
	for _, s := range servers {
		if s.Type == domain.CategorySub {
			sawSub = true
		}
		if s.Type == domain.CategoryDub {
			sawDub = true
		}
	}
	if !sawSub || !sawDub {
		t.Errorf("want both sub and dub servers; got sub=%v dub=%v (%+v)", sawSub, sawDub, servers)
	}
}

// --- doGraphQL transport semantics ---------------------------------------

func TestDoGraphQL_5xxIsProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream timeout"))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Anything"})
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown for 502, got %v", err)
	}
}

func TestDoGraphQL_4xxIsExtractFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["PersistedQueryNotFound"]}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Anything"})
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for 400, got %v", err)
	}
}

// --- HealthCheck ---------------------------------------------------------

func TestHealthCheck_BootSeedsAllStagesUp(t *testing.T) {
	log := logger.Default()
	p, _ := New(Deps{HTTP: domain.NewBaseHTTPClient(log), Cache: newInMemoryCache()})
	h := p.HealthCheck(context.Background())
	if h.Provider != "allanime" {
		t.Fatalf("expected provider=allanime, got %q", h.Provider)
	}
	for _, stage := range stageNames {
		s, ok := h.Stages[stage]
		if !ok {
			t.Fatalf("missing stage %q in HealthCheck snapshot", stage)
		}
		if !s.Up {
			t.Fatalf("expected stage %q Up=true at boot, got %+v", stage, s)
		}
	}
}

func TestSplitEpisodeID(t *testing.T) {
	cases := []struct {
		in      string
		showID  string
		episode string
	}{
		{"abc:1", "abc", "1"},
		{"frieren_show_id:28", "frieren_show_id", "28"},
		{"invalid_no_colon", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		got1, got2 := splitEpisodeID(tc.in)
		if got1 != tc.showID || got2 != tc.episode {
			t.Errorf("splitEpisodeID(%q) = (%q, %q), want (%q, %q)", tc.in, got1, got2, tc.showID, tc.episode)
		}
	}
}

func TestDecodeSourceURL_Passthrough(t *testing.T) {
	in := "https://cdn.example.com/abc.m3u8"
	if out := decodeSourceURL(in); out != in {
		t.Fatalf("expected passthrough, got %q", out)
	}
}

// TestGetStream_HonorsDubCategory verifies that GetStream sends translationType=dub
// upstream when called with domain.CategoryDub.
func TestGetStream_HonorsDubCategory(t *testing.T) {
	var gotDub bool
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".m3u8"):
			// Probe hit: return a real HLS manifest so sourceprobe classifies as Stream.
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n"))
		default:
			// GraphQL API call: capture whether translationType=dub appears.
			raw := r.URL.RawQuery
			if strings.Contains(raw, "dub") || strings.Contains(raw, "%22dub%22") {
				gotDub = true
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"` + srv.URL + `/dub-ep1.m3u8","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"m3u8","subtitles":[]}]}}}`))
		}
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _ = p.GetStream(context.Background(), "", "SHOW123:1", "Default", domain.CategoryDub)
	if !gotDub {
		t.Error("GetStream(dub) did not send translationType=dub upstream")
	}
}

/* removed: host-list tests (TestIsEmbedPageHost_OkRu, TestMaterializeServers_*)
   — embed detection is now content-probe-based; see sourceprobe + the
   GetStream classifier tests above.
func TestIsEmbedPageHost_OkRu(t *testing.T) {
	cases := map[string]bool{
		"https://ok.ru/videoembed/14469506337426": true,
		"https://www.ok.ru/videoembed/123":        true,
		"https://m.ok.ru/videoembed/123":          true,
		"https://www.mp4upload.com/embed-x.html":   true,
		// Subdomain churn must stay covered (suffix match).
		"https://allanime.uns.bio/embed/abc": true,
		"https://uns.bio/embed/abc":          true,
		"https://vidnest.io/e/xyz":           true,
		"https://edge.vidnest.io/e/xyz":      true,
		// Playable direct streams must NOT be flagged.
		"https://wixmp-ed30a86b8c4ca887773594c2.appspot.com/v.m3u8": false,
		"https://tools.fast4speed.rsvp/file/x/master.m3u8":          false,
	}
	for u, want := range cases {
		if got := isEmbedPageHost(u); got != want {
			t.Errorf("isEmbedPageHost(%q) = %v; want %v", u, got, want)
		}
	}
}

// TestMaterializeServers_FiltersEmbedHosts verifies embed-page sources (ok.ru)
// are NOT offered as servers, while playable ones are kept.
func TestMaterializeServers_FiltersEmbedHosts(t *testing.T) {
	sources := []sourceURL{
		{SourceName: "Ok", SourceURL: "https://ok.ru/videoembed/14469506337426", Priority: 9},
		{SourceName: "Default", SourceURL: "https://cdn.example.com/master.m3u8", Priority: 5},
		{SourceName: "Mp4", SourceURL: "https://www.mp4upload.com/embed-abc.html", Priority: 8},
	}
	servers := materializeServers(sources)
	if len(servers) != 1 {
		t.Fatalf("materializeServers kept %d servers; want 1 (only Default)", len(servers))
	}
	if servers[0].ID != "Default" {
		t.Errorf("kept server = %q; want Default (embed hosts must be dropped)", servers[0].ID)
	}
}

// TestMaterializeServers_AllEmbeds_ReturnsEmpty — when every source is an embed
// host, no servers are offered (ListServers then fails over to the next
// provider rather than handing the FE an empty list).
func TestMaterializeServers_AllEmbeds_ReturnsEmpty(t *testing.T) {
	sources := []sourceURL{
		{SourceName: "Ok", SourceURL: "https://ok.ru/videoembed/1"},
		{SourceName: "Ok2", SourceURL: "https://m.ok.ru/videoembed/2"},
	}
	if got := materializeServers(sources); len(got) != 0 {
		t.Errorf("materializeServers(all embeds) = %d servers; want 0", len(got))
	}
}
*/
