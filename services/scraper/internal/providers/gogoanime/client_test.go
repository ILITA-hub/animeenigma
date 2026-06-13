package gogoanime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// goldenPath resolves the path to a Phase 18 golden fixture (Plan 18-01 Task 3
// captured the 8 anitaku.to / embed-wrapper / malsync goldens under
// services/scraper/testdata/gogoanime/). Used by every test below to fail
// loudly if a golden has been deleted between runs.
func goldenPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "gogoanime", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden %s missing: %v", name, err)
	}
	return p
}

// loadFixture reads a file under services/scraper/testdata/gogoanime/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(goldenPath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// fakeMalSync is an in-memory MalSyncClient impl for tests.
type fakeMalSync struct {
	mu       sync.Mutex
	mappings map[string]string // malID → gogoanime slug
	misses   map[string]bool   // malID → confirmed miss
	calls    int
}

func (f *fakeMalSync) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if id, ok := f.mappings[malID]; ok {
		return id, true, nil
	}
	if f.misses[malID] {
		return "", false, nil
	}
	return "", false, nil
}

// fakeStreamExtractor pretends to be one of Plan 18-03's vibeplayer/streamhg/
// earnvids extractors. It matches every URL (Matches always true) and Extract
// returns a configurable Stream so the Provider's GetStream caching path can
// be tested without standing up a real extractor.
type fakeStreamExtractor struct {
	mu      sync.Mutex
	streams map[string]*domain.Stream // embedURL → Stream
	def     *domain.Stream            // fallback when no map entry
	calls   int
}

func (f *fakeStreamExtractor) Name() string         { return "fake-embed" }
func (f *fakeStreamExtractor) Matches(string) bool  { return true }

func (f *fakeStreamExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if s, ok := f.streams[embedURL]; ok {
		return s, nil
	}
	if f.def != nil {
		return f.def, nil
	}
	return &domain.Stream{Sources: []domain.Source{{URL: embedURL + "#m3u8", Type: "hls"}}}, nil
}

var _ domain.EmbedExtractor = (*fakeStreamExtractor)(nil)

// newTestProvider wires a Provider to an httptest.Server (or the same server
// for any path the test handler routes). Retries disabled for deterministic
// counts.
func newTestProvider(t *testing.T, srv *httptest.Server) (*Provider, *fakeCache, *fakeMalSync, *fakeStreamExtractor) {
	t.Helper()
	log := newTestLogger(t)
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	fc := newFakeCache()
	fm := &fakeMalSync{
		mappings: map[string]string{},
		misses:   map[string]bool{},
	}
	fk := &fakeStreamExtractor{streams: map[string]*domain.Stream{}}
	reg := domain.NewRegistry()
	reg.Register(fk)

	p, err := New(Deps{
		BaseURL: srv.URL,
		HTTP:    hc,
		Embeds:  reg,
		MalSync: fm,
		Cache:   fc,
		Log:     log,
	})
	if err != nil {
		t.Fatalf("New(Deps{...}) = err %v; want nil", err)
	}
	return p, fc, fm, fk
}

// TestProvider_Name pins the stable slug literal.
func TestProvider_Name(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	if got := p.Name(); got != "gogoanime" {
		t.Errorf("Name() = %q; want %q", got, "gogoanime")
	}
}

// TestFindID_FuzzyPath verifies SCRAPER-9ANI-01: malsync returns the steady-
// state miss; fuzzy /search.html is the PRIMARY resolution path. With the
// captured search_attack_on_titan.html golden, ref.Title="Attack on Titan"
// must resolve to slug "attack-on-titan" (exact-match top-row).
func TestFindID_FuzzyPath(t *testing.T) {
	t.Parallel()
	html := loadFixture(t, "search_attack_on_titan.html")
	var searchCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchCalls.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/search.html") {
			t.Errorf("unexpected path %q; want /search.html", r.URL.Path)
		}
		if q := r.URL.Query().Get("keyword"); q != "Attack on Titan" {
			t.Errorf("keyword= %q; want \"Attack on Titan\"", q)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(html)
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	id, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "16498", Title: "Attack on Titan"})
	if err != nil {
		t.Fatalf("FindID err = %v; want nil", err)
	}
	if id != "attack-on-titan" {
		t.Errorf("FindID = %q; want \"attack-on-titan\"", id)
	}
	if searchCalls.Load() == 0 {
		t.Errorf("upstream /search.html was never called")
	}
	// HealthCheck reflects the search-stage success.
	h := p.HealthCheck(context.Background())
	if !h.Stages[health_StageSearch()].Up {
		t.Errorf("StageSearch.Up = false; want true after successful FindID")
	}
}

// TestFindID_MalsyncNegativeCache verifies SCRAPER-9ANI-01: a malsync miss
// flows through to the fuzzy search; the fakeMalSync misses path is exercised
// without crashing the lookup.
func TestFindID_MalsyncNegativeCache(t *testing.T) {
	t.Parallel()
	html := loadFixture(t, "search_attack_on_titan.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(html)
	}))
	defer srv.Close()
	p, _, fm, _ := newTestProvider(t, srv)
	// Pre-populate fakeMalSync misses so Lookup returns ("",false,nil) cheaply
	// (the real client would have negative-cached this via malsync_no_gogo.json
	// — fakeMalSync just short-circuits in-memory).
	fm.misses["16498"] = true

	id, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "16498", Title: "Attack on Titan"})
	if err != nil {
		t.Fatalf("FindID err = %v", err)
	}
	if id != "attack-on-titan" {
		t.Errorf("FindID = %q; want \"attack-on-titan\"", id)
	}
	// Malsync Lookup MUST have been called (forward-compat probe still fires
	// — the negative cache is what malsync.MalSyncClient consults internally).
	if fm.calls != 1 {
		t.Errorf("fakeMalSync.calls = %d; want 1 (one forward-compat probe per FindID)", fm.calls)
	}
}

// TestListEpisodes_SubDubMerge verifies SCRAPER-9ANI-02: ListEpisodes fetches
// both /category/<base> (sub) and /category/<base>-dub (which 404s for One
// Piece in the captured golden), returns the merged list, and at least one
// sub-tagged episode is present. (The captured dub golden is a 404 page so
// the dub-tagged assertion isn't applicable to THIS fixture pair, but the
// merge path is still exercised — the soft-404 detector keeps it from being
// an error.)
func TestListEpisodes_SubDubMerge(t *testing.T) {
	t.Parallel()
	subHTML := loadFixture(t, "category_one_piece.html")
	dubHTML := loadFixture(t, "category_one_piece_dub.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/category/one-piece":
			_, _ = w.Write(subHTML)
		case "/category/one-piece-dub":
			// Golden is the soft-404 "Pages not found" page; ListEpisodes
			// should still detect this and gracefully proceed with sub-only.
			_, _ = w.Write(dubHTML)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	eps, err := p.ListEpisodes(context.Background(), "one-piece")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil", err)
	}
	if len(eps) < 100 {
		t.Fatalf("len(eps) = %d; want >= 100 (One Piece anitaku.to golden)", len(eps))
	}
	// Episode 1 should be present and use the sub URL slug (no "-dub-").
	var ep1 *domain.Episode
	for i := range eps {
		if eps[i].Number == 1 {
			ep1 = &eps[i]
			break
		}
	}
	if ep1 == nil {
		t.Fatal("expected episode 1 in merged result")
	}
	if strings.Contains(ep1.ID, "-dub-") {
		t.Errorf("episode 1 ID = %q; want a sub slug (no -dub-)", ep1.ID)
	}
	// Health stage flipped to up.
	h := p.HealthCheck(context.Background())
	if !h.Stages[health_StageEpisodes()].Up {
		t.Errorf("StageEpisodes.Up = false; want true")
	}
}

// TestListEpisodes_CacheHit verifies SCRAPER-9ANI-02: the second call (with
// the upstream now returning 500) returns the cached result without HTTP.
func TestListEpisodes_CacheHit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	subHTML := loadFixture(t, "category_one_piece.html")
	dubHTML := loadFixture(t, "category_one_piece_dub.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := calls.Add(1)
		if c > 2 {
			// After the first sub+dub round, fail loudly.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/category/one-piece":
			_, _ = w.Write(subHTML)
		case "/category/one-piece-dub":
			_, _ = w.Write(dubHTML)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	first, err := p.ListEpisodes(context.Background(), "one-piece")
	if err != nil {
		t.Fatalf("first ListEpisodes err = %v", err)
	}
	callsBefore := calls.Load()
	second, err := p.ListEpisodes(context.Background(), "one-piece")
	if err != nil {
		t.Fatalf("second ListEpisodes err = %v", err)
	}
	callsAfter := calls.Load()
	if callsAfter != callsBefore {
		t.Errorf("upstream calls increased on cache hit: before=%d after=%d", callsBefore, callsAfter)
	}
	if len(second) != len(first) {
		t.Errorf("cached len = %d; first len = %d", len(second), len(first))
	}
}

// TestListServers_AnimeMutiLink verifies SCRAPER-9ANI-03: ListServers parses
// the .anime_muti_link block on the captured one_piece_episode_1.html golden
// and returns at least one entry for each canonical host (vibeplayer,
// otakuhg, otakuvid). All returned servers are sub-tagged.
func TestListServers_AnimeMutiLink(t *testing.T) {
	t.Parallel()
	html := loadFixture(t, "one_piece_episode_1.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/one-piece-episode-1" {
			t.Errorf("unexpected path %q; want /one-piece-episode-1", r.URL.Path)
		}
		_, _ = w.Write(html)
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	servers, err := p.ListServers(context.Background(), "one-piece", "one-piece-episode-1")
	if err != nil {
		t.Fatalf("ListServers err = %v", err)
	}
	if len(servers) < 3 {
		t.Fatalf("len(servers) = %d; want >= 3", len(servers))
	}
	// Collect hosts to assert the canonical embed targets are present.
	hosts := make(map[string]bool)
	for _, s := range servers {
		if u, err := url.Parse(s.ID); err == nil {
			hosts[strings.ToLower(u.Hostname())] = true
		}
		if s.Type != domain.CategorySub {
			t.Errorf("server %q type = %v; want CategorySub (slug is sub)", s.Name, s.Type)
		}
	}
	for _, want := range []string{"vibeplayer.site", "otakuhg.site", "otakuvid.online"} {
		if !hosts[want] {
			t.Errorf("expected host %q in server list; got hosts=%v", want, hosts)
		}
	}
}

// TestListServers_DoodstreamSkipped verifies SCRAPER-9ANI-03: the
// Cloudflare-Turnstile-gated embed hosts (myvidplay.com / playmogo.com) are
// filtered out at ListServers time. The captured golden has multiple
// myvidplay.com entries (verified at capture time).
func TestListServers_DoodstreamSkipped(t *testing.T) {
	t.Parallel()
	html := loadFixture(t, "one_piece_episode_1.html")
	// Sanity-check the golden has Doodstream entries; if the fixture is
	// refreshed and the host disappears, the test should fail loudly rather
	// than silently rubber-stamp.
	if !strings.Contains(string(html), "myvidplay.com") {
		t.Fatal("golden one_piece_episode_1.html no longer contains myvidplay.com — refresh fixture or remove this test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(html)
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	servers, err := p.ListServers(context.Background(), "one-piece", "one-piece-episode-1")
	if err != nil {
		t.Fatalf("ListServers err = %v", err)
	}
	for _, s := range servers {
		u, err := url.Parse(s.ID)
		if err != nil {
			t.Errorf("server.ID %q is not parseable: %v", s.ID, err)
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "myvidplay.com" || strings.HasSuffix(host, ".myvidplay.com") {
			t.Errorf("Doodstream/myvidplay.com leaked through filter: %s", s.ID)
		}
		if host == "playmogo.com" || strings.HasSuffix(host, ".playmogo.com") {
			t.Errorf("playmogo.com leaked through filter: %s", s.ID)
		}
	}
}

// TestListServers_UnextractableHostsSkipped verifies AUTO-459: embed hosts
// with no registered extractor (vidmoly.biz/.net, filemoon.sx,
// bysesayeveum.com) are filtered out at ListServers time so they never reach
// GetStream and never burn the cold-path budget as cdn_unreachable noise. A
// co-located extractable host (vibeplayer.site) must still survive the filter.
func TestListServers_UnextractableHostsSkipped(t *testing.T) {
	t.Parallel()
	// Inline episode page: one extractable host + every unextractable host,
	// plus a strict-subdomain of one to exercise the suffix match.
	const page = `<html><body><div class="anime_muti_link"><ul class="muti_link">
<li><a data-video="https://vibeplayer.site/e/keep1" rel="1">Vidstreaming Choose this server</a></li>
<li><a data-video="https://vidmoly.biz/embed-abc.html" rel="1">Vidmoly</a></li>
<li><a data-video="https://vidmoly.net/embed-def.html" rel="1">Vidmoly</a></li>
<li><a data-video="https://filemoon.sx/e/ghi" rel="1">Filemoon</a></li>
<li><a data-video="https://bysesayeveum.com/e/jkl" rel="1">Streamwish</a></li>
<li><a data-video="https://cdn.vidmoly.biz/embed-sub.html" rel="1">Vidmoly sub</a></li>
</ul></div></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	servers, err := p.ListServers(context.Background(), "demo", "demo-episode-1")
	if err != nil {
		t.Fatalf("ListServers err = %v", err)
	}
	banned := []string{"vidmoly.biz", "vidmoly.net", "filemoon.sx", "bysesayeveum.com"}
	for _, s := range servers {
		u, perr := url.Parse(s.ID)
		if perr != nil {
			t.Errorf("server.ID %q not parseable: %v", s.ID, perr)
			continue
		}
		host := strings.ToLower(u.Hostname())
		for _, b := range banned {
			if host == b || strings.HasSuffix(host, "."+b) {
				t.Errorf("unextractable host %q leaked through filter: %s", b, s.ID)
			}
		}
	}
	// The one extractable host must survive (filter must not over-match).
	if len(servers) != 1 {
		t.Fatalf("len(servers) = %d; want 1 (only vibeplayer.site survives)", len(servers))
	}
	if got := strings.ToLower(mustHost(t, servers[0].ID)); got != "vibeplayer.site" {
		t.Errorf("surviving server host = %q; want vibeplayer.site", got)
	}
}

// mustHost parses rawURL and returns its hostname, failing the test on error.
func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return u.Hostname()
}

// TestGetStream_DispatchesToRegistry verifies SCRAPER-9ANI-04: GetStream
// looks up the embed URL via embeds.Registry.Find(serverID) and returns the
// extractor's *Stream. No extraction logic in this provider.
func TestGetStream_DispatchesToRegistry(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, fk := newTestProvider(t, srv)
	serverID := "https://vibeplayer.site/abc?sub=https://example.com/sub.vtt"
	want := &domain.Stream{
		Sources: []domain.Source{{URL: "https://example.test/playlist.m3u8", Type: "hls"}},
		Headers: map[string]string{"Referer": "https://anitaku.to/"},
	}
	fk.streams[serverID] = want

	stream, err := p.GetStream(context.Background(), "one-piece", "one-piece-episode-1", serverID, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatalf("GetStream returned empty stream")
	}
	if stream.Sources[0].URL != want.Sources[0].URL {
		t.Errorf("source URL = %q; want %q", stream.Sources[0].URL, want.Sources[0].URL)
	}
	h := p.HealthCheck(context.Background())
	if !h.Stages[health_StageStream()].Up {
		t.Errorf("StageStream.Up = false; want true")
	}
}

// TestGetStream_StreamTTL verifies SCRAPER-9ANI-04: stream URL caching uses
// computeStreamTTL with the StreamHG/Earnvids &e=<delta_seconds> semantics
// (paired with &s=<unix_signed_at> for absolute expiry).
func TestGetStream_StreamTTL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, _, fk := newTestProvider(t, srv)
	// &e=300 and a future s= → headroom ~ 270s, well under the 5min cap.
	signedAt := time.Now().Unix()
	streamURL := "https://stream.example.com/playlist.m3u8?s=" +
		intToStr(signedAt) + "&e=300"
	serverID := "https://vibeplayer.site/abc"
	fk.def = &domain.Stream{Sources: []domain.Source{{URL: streamURL, Type: "hls"}}}

	_, err := p.GetStream(context.Background(), "one-piece", "one-piece-episode-1", serverID, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	// Cache write should have fired: stream:gogoanime:<slug>:<epID>:<hash>.
	wroteStream := false
	for _, k := range fc.snapshotSetLog() {
		if strings.HasPrefix(k, "stream:gogoanime:one-piece:one-piece-episode-1:") {
			wroteStream = true
			break
		}
	}
	if !wroteStream {
		t.Errorf("expected cache write under stream:gogoanime:*; setLog=%v", fc.snapshotSetLog())
	}
}

// Tiny helper to avoid importing strconv just for one line in TestGetStream_StreamTTL.
func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestNew_RequiresDependencies anchors the WR-11 pattern — every required
// Deps field must error out at construction rather than nil-panic at first
// use.
func TestNew_RequiresDependencies(t *testing.T) {
	t.Parallel()
	log := newTestLogger(t)
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	reg := domain.NewRegistry()
	fm := &fakeMalSync{mappings: map[string]string{}, misses: map[string]bool{}}
	fc := newFakeCache()

	cases := []struct {
		name string
		deps Deps
		want string
	}{
		{name: "missing HTTP", deps: Deps{Embeds: reg, MalSync: fm, Cache: fc, Log: log}, want: "HTTP"},
		{name: "missing Embeds", deps: Deps{HTTP: hc, MalSync: fm, Cache: fc, Log: log}, want: "Embeds"},
		{name: "missing MalSync", deps: Deps{HTTP: hc, Embeds: reg, Cache: fc, Log: log}, want: "MalSync"},
		{name: "missing Cache", deps: Deps{HTTP: hc, Embeds: reg, MalSync: fm, Log: log}, want: "Cache"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := New(tc.deps)
			if err == nil {
				t.Fatalf("New(%s) = nil err; want error", tc.name)
			}
			if p != nil {
				t.Errorf("New(%s) returned non-nil Provider on error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

// TestFindID_NoMatch verifies the WrapNotFound error path when the search
// page returns zero results.
func TestFindID_NoMatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div class="last_episodes"><ul></ul></div></body></html>`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "NonexistentXYZ"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindID err = %v; want ErrNotFound", err)
	}
}

// Helper indirections to expose the health-package constants without an
// extra import in this file. Tests use these so a stage-key rename in the
// canonical health package is caught by the compiler here too.
func health_StageSearch() string   { return stageNames[0] }
func health_StageEpisodes() string { return stageNames[1] }
func health_StageServers() string  { return stageNames[2] }
func health_StageStream() string   { return stageNames[3] }

// TestSearchKeywords verifies the mirror-safe keyword derivation: gogoanimes.fi
// matches the keyword as a literal substring and 404s on apostrophes, so we emit
// the leading clean phrase (up to first punctuation) + a first-word fallback.
func TestSearchKeywords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		title string
		want  []string
	}{
		{"Frieren: Beyond Journey's End", []string{"Frieren"}},
		{"Re:Zero kara Hajimeru Isekai Seikatsu", []string{"Re"}},
		{"Dr. Stone", []string{"Dr"}},
		{"One Piece", []string{"One Piece", "One"}},
		{"Sousou no Frieren", []string{"Sousou no Frieren", "Sousou"}},
		{"JoJo's Bizarre Adventure", []string{"JoJo"}},
		{"   ", nil},
	}
	for _, c := range cases {
		got := searchKeywords(c.title)
		if len(got) != len(c.want) {
			t.Errorf("searchKeywords(%q) = %v; want %v", c.title, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("searchKeywords(%q)[%d] = %q; want %q", c.title, i, got[i], c.want[i])
			}
		}
	}
}
