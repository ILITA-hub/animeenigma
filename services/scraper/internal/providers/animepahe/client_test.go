package animepahe

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// fakeMalSync is an in-memory MalSyncClient impl for tests.
//
// Phase 27 A9: extended with reverse-mapping (providerID → malID) +
// Invalidate so the parser's /release 404 single-strike path can be
// exercised without standing up a real Redis. The reverse map is
// populated automatically when a successful Lookup fires (matches
// production behavior).
type fakeMalSync struct {
	mu       sync.Mutex
	mappings map[string]string // malID → animepahe providerID
	reverse  map[string]string // providerID → malID (populated on Lookup hit)
	misses   map[string]bool   // malID → confirmed miss
	calls    int
}

func (f *fakeMalSync) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if id, ok := f.mappings[malID]; ok {
		if f.reverse == nil {
			f.reverse = map[string]string{}
		}
		f.reverse[id] = malID
		return id, true, nil
	}
	if f.misses[malID] {
		return "", false, nil
	}
	return "", false, nil
}

// LookupMalID is the reverse of Lookup. Returns ("", nil) on unknown
// providerID — matches production MalSyncClient.LookupMalID semantics.
func (f *fakeMalSync) LookupMalID(ctx context.Context, providerID, provider string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reverse == nil {
		return "", nil
	}
	if mal, ok := f.reverse[providerID]; ok {
		return mal, nil
	}
	return "", nil
}

// Invalidate drops both directions of the mapping (single-strike A9).
func (f *fakeMalSync) Invalidate(ctx context.Context, malID, provider, providerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.mappings, malID)
	if f.reverse != nil {
		delete(f.reverse, providerID)
	}
	return nil
}

// fakeKwikExtractor pretends to be the Plan 16-02 KwikExtractor. Matches kwik
// hosts (substring on host suffix for simplicity in tests) and Extract returns
// a canned Stream so the Provider's GetStream caching path can be tested
// without standing up a real httptest kwik server.
//
// Phase 27: lastReferer captures the Referer header passed by GetStream so
// TestProvider_GetStream_HappyPath can assert it equals the kwikReferer
// constant (`https://animepahe.pw/`) — pins the B1 contract.
type fakeKwikExtractor struct {
	mu          sync.Mutex
	streams     map[string]*domain.Stream // embedURL → Stream
	calls       int
	lastReferer string
}

func (f *fakeKwikExtractor) Name() string { return "kwik" }
func (f *fakeKwikExtractor) Matches(u string) bool {
	pu, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := strings.ToLower(pu.Hostname())
	return host == "kwik.cx" || strings.HasSuffix(host, ".kwik.cx") ||
		host == "kwik.si" || strings.HasSuffix(host, ".kwik.si")
}

func (f *fakeKwikExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastReferer = headers.Get("Referer")
	if s, ok := f.streams[embedURL]; ok {
		return s, nil
	}
	return &domain.Stream{
		Sources: []domain.Source{{URL: embedURL + "#m3u8", Type: "hls"}},
		Headers: map[string]string{"Referer": "https://kwik.cx/"},
	}, nil
}

var _ domain.EmbedExtractor = (*fakeKwikExtractor)(nil)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// newTestProvider builds a Provider wired to an httptest.Server. The HTTP
// client has retries disabled so test assertions on call counts are stable.
func newTestProvider(t *testing.T, srv *httptest.Server) (*Provider, *fakeCache, *fakeMalSync, *fakeKwikExtractor) {
	t.Helper()
	log := newTestLogger(t)
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	fc := newFakeCache()
	fm := &fakeMalSync{
		mappings: map[string]string{},
		misses:   map[string]bool{},
	}
	fk := &fakeKwikExtractor{streams: map[string]*domain.Stream{}}
	reg := domain.NewRegistry()
	reg.Register(fk)

	resolverURL := srv.URL
	p, err := New(Deps{
		ResolverURL: resolverURL,
		HTTP:        hc,
		Embeds:      reg,
		MalSync:     fm,
		Cache:       fc,
		Log:         log,
	})
	if err != nil {
		t.Fatalf("New(Deps{...}) = err %v; want nil", err)
	}
	return p, fc, fm, fk
}

// loadFixture reads a file under services/scraper/testdata/animepahe/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "animepahe", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestProvider_Name: stable identifier "animepahe".
func TestProvider_Name(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	if got := p.Name(); got != "animepahe" {
		t.Errorf("Name() = %q; want %q", got, "animepahe")
	}
}

// TestProvider_FindID_MalSyncHit: malsync returns a hit → provider uses it
// and does NOT call the upstream search API.
func TestProvider_FindID_MalSyncHit(t *testing.T) {
	t.Parallel()
	var searchCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	p, _, fm, _ := newTestProvider(t, srv)
	fm.mappings["21"] = "1"

	id, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "21", Title: "One Piece"})
	if err != nil {
		t.Fatalf("FindID err = %v", err)
	}
	if id != "1" {
		t.Errorf("FindID = %q; want %q", id, "1")
	}
	if searchCalls.Load() != 0 {
		t.Errorf("must not call upstream search on malsync hit; got %d calls", searchCalls.Load())
	}
}

// TestProvider_FindID_FuzzyFallback: malsync miss → search returns 3 entries
// with title "Naruto" topping the score; FindID returns its session.
func TestProvider_FindID_FuzzyFallback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("expected /search; got %s", r.URL.Path)
		}
		_, _ = w.Write(loadFixture(t, "search_naruto.json"))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	id, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "20", Title: "Naruto"})
	if err != nil {
		t.Fatalf("FindID err = %v", err)
	}
	if id != "anime-session-naruto-001" {
		t.Errorf("FindID = %q; want \"anime-session-naruto-001\"", id)
	}
}

// TestProvider_FindID_NoMatch: malsync miss AND search returns zero entries →
// wrapped ErrNotFound.
func TestProvider_FindID_NoMatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total":0,"data":[]}`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Nonexistent"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindID err = %v; want ErrNotFound", err)
	}
}

// TestProvider_FindID_FuzzyBelowThreshold: search returns entries whose
// best Jaro-Winkler score is below 0.85 → wrapped ErrNotFound.
func TestProvider_FindID_FuzzyBelowThreshold(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Entries with low similarity to the search query "Naruto".
		_, _ = w.Write([]byte(`{"total":1,"data":[{"id":99,"session":"sx","title":"Completely Different Show","type":"TV","year":2020,"episodes":12}]}`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Naruto"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindID err = %v; want ErrNotFound", err)
	}
}

// TestProvider_ListEpisodes_SinglePage: returns mapped Episodes from
// release_4_p1.json and writes the assembled list to the 6h cache.
func TestProvider_ListEpisodes_SinglePage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release" {
			t.Errorf("expected /release; got %s", r.URL.Path)
		}
		_, _ = w.Write(loadFixture(t, "release_4_p1.json"))
	}))
	defer srv.Close()
	p, fc, _, _ := newTestProvider(t, srv)

	eps, err := p.ListEpisodes(context.Background(), "4")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v", err)
	}
	if len(eps) != 5 {
		t.Fatalf("len(eps) = %d; want 5", len(eps))
	}
	// Spot-check the first and an in-fixture filler episode.
	if eps[0].Number != 1 {
		t.Errorf("eps[0].Number = %d; want 1", eps[0].Number)
	}
	if eps[0].ID != "ep-session-abc-001" {
		t.Errorf("eps[0].ID = %q; want ep-session-abc-001", eps[0].ID)
	}
	if !eps[2].IsFiller {
		t.Errorf("eps[2].IsFiller should be true (fixture marks ep 3 as filler)")
	}
	// Cache write should target episodes:animepahe:4.
	foundCache := false
	for _, k := range fc.snapshotSetLog() {
		if k == "episodes:animepahe:4" {
			foundCache = true
		}
	}
	if !foundCache {
		t.Errorf("expected cache write to episodes:animepahe:4; got %v", fc.snapshotSetLog())
	}
}

// TestProvider_ListEpisodes_Pagination: 3-page fixture → all 3 page calls
// fire in order; assembled list concatenates all pages.
func TestProvider_ListEpisodes_Pagination(t *testing.T) {
	t.Parallel()
	type epBlock struct {
		current, last int
		eps           []int
	}
	pages := map[string]epBlock{
		"1": {1, 3, []int{1, 2}},
		"2": {2, 3, []int{3, 4}},
		"3": {3, 3, []int{5, 6}},
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		blk, ok := pages[page]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`{"current_page":%d,"last_page":%d,"data":[`, blk.current, blk.last))
		for i, n := range blk.eps {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf(`{"session":"s%d","episode":%d,"filler":0}`, n, n))
		}
		sb.WriteString("]}")
		_, _ = w.Write([]byte(sb.String()))
	}))
	defer srv.Close()

	p, _, _, _ := newTestProvider(t, srv)
	eps, err := p.ListEpisodes(context.Background(), "4")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v", err)
	}
	if len(eps) != 6 {
		t.Errorf("len(eps) = %d; want 6", len(eps))
	}
	if calls.Load() != 3 {
		t.Errorf("upstream calls = %d; want 3", calls.Load())
	}
	for i, ep := range eps {
		if ep.Number != i+1 {
			t.Errorf("eps[%d].Number = %d; want %d", i, ep.Number, i+1)
		}
	}
}

// TestProvider_ListEpisodes_CacheHit: pre-populate cache → ListEpisodes
// returns cached value without HTTP.
func TestProvider_ListEpisodes_CacheHit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	p, fc, _, _ := newTestProvider(t, srv)
	cached := []domain.Episode{{ID: "s1", Number: 1}, {ID: "s2", Number: 2}}
	if err := fc.Set(context.Background(), "episodes:animepahe:9", cached, 6*time.Hour); err != nil {
		t.Fatal(err)
	}
	eps, err := p.ListEpisodes(context.Background(), "9")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v", err)
	}
	if len(eps) != 2 {
		t.Errorf("len(eps) = %d; want 2", len(eps))
	}
	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d; want 0", calls.Load())
	}
}

// TestProvider_ListEpisodes_RealEmpty: upstream returns 1-page, 0-data →
// returns ([]Episode{}, nil) NOT an error.
func TestProvider_ListEpisodes_RealEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"current_page":1,"last_page":1,"data":[]}`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	eps, err := p.ListEpisodes(context.Background(), "1234")
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if eps == nil {
		t.Fatal("eps should be []Episode{}, not nil")
	}
	if len(eps) != 0 {
		t.Errorf("len(eps) = %d; want 0", len(eps))
	}
}

// TestProvider_ListEpisodes_Upstream5xx: upstream 503 → wrapped ErrProviderDown.
func TestProvider_ListEpisodes_Upstream5xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	_, err := p.ListEpisodes(context.Background(), "x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want ErrProviderDown", err)
	}
}

// TestProvider_ListServers_HappyPath: parses play_session_ep1.html, returns
// one Server per kwik data-src URL.
func TestProvider_ListServers_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/play" {
			t.Errorf("expected /play (resolver shape); got %s", r.URL.Path)
		}
		if r.URL.Query().Get("animeSession") == "" || r.URL.Query().Get("episodeSession") == "" {
			t.Errorf("expected animeSession + episodeSession query params; got %s", r.URL.RawQuery)
		}
		_, _ = w.Write(loadFixture(t, "play_session_ep1.html"))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	servers, err := p.ListServers(context.Background(), "anime-session", "ep-session")
	if err != nil {
		t.Fatalf("ListServers err = %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("len(servers) = %d; want 3", len(servers))
	}
	subCount := 0
	for _, s := range servers {
		if !strings.HasPrefix(s.ID, "https://kwik.cx/") {
			t.Errorf("server.ID should be a kwik URL; got %q", s.ID)
		}
		if s.Name != "kwik" {
			t.Errorf("server.Name = %q; want \"kwik\"", s.Name)
		}
		// CR-02: the fixture buttons all carry data-audio="jpn", so every
		// server entry should be tagged CategorySub. The frontend's
		// `subServers.filter(s => s.type === 'sub')` would otherwise produce
		// an empty list.
		if s.Type == domain.CategorySub {
			subCount++
		}
	}
	if subCount != 3 {
		t.Errorf("expected 3 sub-tagged servers; got %d", subCount)
	}
}

// TestProvider_ListServers_DubAudio: a play page with data-audio="eng" maps
// to CategoryDub. Anchors CR-02's contract.
func TestProvider_ListServers_DubAudio(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<button data-src="https://kwik.cx/e/dub-720p" data-audio="eng" data-resolution="720">EngDub · 720p</button>
<button data-src="https://kwik.cx/e/sub-720p" data-audio="jpn" data-resolution="720">JpSub · 720p</button>
</body></html>`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	servers, err := p.ListServers(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("ListServers err = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d; want 2", len(servers))
	}
	var sawSub, sawDub bool
	for _, s := range servers {
		switch s.Type {
		case domain.CategorySub:
			sawSub = true
		case domain.CategoryDub:
			sawDub = true
		}
	}
	if !sawSub || !sawDub {
		t.Errorf("want both sub and dub server entries; got sub=%v dub=%v", sawSub, sawDub)
	}
}

// TestProvider_ListServers_NoButtons: HTML with no data-src buttons →
// ([]Server{}, nil) NOT error (real-empty per domain contract).
func TestProvider_ListServers_NoButtons(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>No streams available</h1><p>Sorry, this episode has no kwik buttons.</p></body></html>`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	servers, err := p.ListServers(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if servers == nil {
		t.Fatal("servers should be []Server{}, not nil")
	}
	if len(servers) != 0 {
		t.Errorf("len(servers) = %d; want 0", len(servers))
	}
}

// TestProvider_ListServers_SelectorDrift: empty/malformed body → wrapped
// ErrExtractFailed.
func TestProvider_ListServers_SelectorDrift(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Empty body is the cleanest "selector drift" signal — there's literally
		// no HTML to scrape, so the provider must distinguish this from
		// real-empty.
		_, _ = w.Write([]byte(``))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	_, err := p.ListServers(context.Background(), "a", "b")
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want ErrExtractFailed", err)
	}
}

// TestProvider_GetStream_HappyPath: kwik URL is in the play HTML; registry's
// fake Kwik extractor returns a known Stream; result is cached.
func TestProvider_GetStream_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, _, fk := newTestProvider(t, srv)
	kwikURL := "https://kwik.cx/e/abc-720p?expires=99999999999"
	wantStream := &domain.Stream{
		Sources: []domain.Source{{URL: "https://stream/m3u8?expires=99999999999", Type: "hls", Quality: "720p"}},
		Headers: map[string]string{"Referer": "https://kwik.cx/"},
	}
	fk.streams[kwikURL] = wantStream

	stream, err := p.GetStream(context.Background(), "anime-sess", "ep-sess", kwikURL, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatalf("GetStream returned empty stream")
	}
	if stream.Sources[0].URL != wantStream.Sources[0].URL {
		t.Errorf("source url = %q; want %q", stream.Sources[0].URL, wantStream.Sources[0].URL)
	}
	// Phase 27 B1: Referer passed to the Kwik extractor MUST be the
	// `kwikReferer` constant value (https://animepahe.pw/). Pins the
	// Plan 27-02 Task 1 contract: the Referer source switched from the
	// deleted Provider.baseURL field to the new package-level constant.
	if got, want := fk.lastReferer, kwikReferer; got != want {
		t.Errorf("Kwik extractor Referer = %q; want %q (kwikReferer constant)", got, want)
	}
	if got, want := fk.lastReferer, "https://animepahe.pw/"; got != want {
		t.Errorf("Kwik extractor Referer (literal) = %q; want %q (D2 alignment)", got, want)
	}
	// Cache should contain a stream:animepahe:anime-sess:ep-sess:* key.
	found := false
	for _, k := range fc.snapshotSetLog() {
		if strings.HasPrefix(k, "stream:animepahe:anime-sess:ep-sess:") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stream cache write; setLog=%v", fc.snapshotSetLog())
	}
}

// TestProvider_GetStream_NoExtractor: registry has no matching extractor →
// wrapped ErrExtractFailed.
func TestProvider_GetStream_NoExtractor(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	// Override the registry to have NO extractors.
	p.embeds = domain.NewRegistry()

	_, err := p.GetStream(context.Background(), "a", "b", "https://kwik.cx/e/abc", domain.CategorySub)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want ErrExtractFailed", err)
	}
}

// TestProvider_GetStream_CacheTTL_Capped: expires=now+10min → TTL = 5min.
func TestProvider_GetStream_CacheTTL_Capped(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, _, fk := newTestProvider(t, srv)
	expires := time.Now().Add(10 * time.Minute).Unix()
	kwikURL := fmt.Sprintf("https://kwik.cx/e/abc?expires=%d", expires)
	streamURL := fmt.Sprintf("https://stream/m3u8?expires=%d", expires)
	fk.streams[kwikURL] = &domain.Stream{Sources: []domain.Source{{URL: streamURL, Type: "hls"}}}

	_, err := p.GetStream(context.Background(), "x", "y", kwikURL, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	// We can't inspect the TTL from the fakeCache directly, but we can verify
	// the cache entry exists (i.e. TTL was > 0 ⇒ cache wrote).
	foundCacheWrite := false
	for _, k := range fc.snapshotSetLog() {
		if strings.HasPrefix(k, "stream:animepahe:x:y:") {
			foundCacheWrite = true
		}
	}
	if !foundCacheWrite {
		t.Errorf("expected cache write for capped-TTL stream; setLog=%v", fc.snapshotSetLog())
	}
}

// TestProvider_GetStream_CacheTTL_NoExpiresParam: no expires param → TTL = 5min
// fallback, cache still writes.
func TestProvider_GetStream_CacheTTL_NoExpiresParam(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, _, fk := newTestProvider(t, srv)
	kwikURL := "https://kwik.cx/e/abc-no-expires"
	fk.streams[kwikURL] = &domain.Stream{Sources: []domain.Source{{URL: "https://stream/m3u8", Type: "hls"}}}

	_, err := p.GetStream(context.Background(), "a", "b", kwikURL, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	foundCacheWrite := false
	for _, k := range fc.snapshotSetLog() {
		if strings.HasPrefix(k, "stream:animepahe:a:b:") {
			foundCacheWrite = true
		}
	}
	if !foundCacheWrite {
		t.Errorf("expected cache write for fallback-TTL stream; setLog=%v", fc.snapshotSetLog())
	}
}

// TestProvider_GetStream_ExpiredURL_NoCacheWrite: expires in the past → TTL=0,
// provider must NOT cache (the cached URL would just be a known-bad URL).
func TestProvider_GetStream_ExpiredURL_NoCacheWrite(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, _, fk := newTestProvider(t, srv)
	expires := time.Now().Add(-1 * time.Hour).Unix()
	kwikURL := fmt.Sprintf("https://kwik.cx/e/abc?expires=%d", expires)
	streamURL := fmt.Sprintf("https://stream/m3u8?expires=%d", expires)
	fk.streams[kwikURL] = &domain.Stream{Sources: []domain.Source{{URL: streamURL, Type: "hls"}}}

	_, err := p.GetStream(context.Background(), "x", "y", kwikURL, domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err = %v", err)
	}
	for _, k := range fc.snapshotSetLog() {
		if strings.HasPrefix(k, "stream:animepahe:x:y:") {
			t.Errorf("must NOT cache an expired URL; setLog=%v", fc.snapshotSetLog())
		}
	}
}

// TestProvider_HealthCheck: returns Health with provider="animepahe" and
// the four canonical stage keys.
//
// Phase 17 Plan 02 — keys renamed from the legacy {find_id, list_episodes,
// list_servers, get_stream} to the canonical 5-stage strings owned by
// services/scraper/internal/health/stage.go. The fifth stage (stream_segment)
// is owned by the probe runner, NOT this provider, so it does NOT appear here.
func TestProvider_HealthCheck(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	h := p.HealthCheck(context.Background())
	if h.Provider != "animepahe" {
		t.Errorf("Provider = %q; want \"animepahe\"", h.Provider)
	}
	wantStages := []string{"search", "episodes", "servers", "stream"}
	for _, s := range wantStages {
		if _, ok := h.Stages[s]; !ok {
			t.Errorf("missing canonical stage %q in health snapshot; got keys=%v", s, mapKeys(h.Stages))
		}
	}
	// Negative — legacy keys must be gone.
	legacy := []string{"find_id", "list_episodes", "list_servers", "get_stream"}
	for _, s := range legacy {
		if _, ok := h.Stages[s]; ok {
			t.Errorf("legacy stage key %q still present in health snapshot", s)
		}
	}
}

// mapKeys returns the keys of a string-keyed map (for error messages).
func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestProvider_FindID_QuerySafetyEscape: a ref.Title with shell/regex
// metacharacters (e.g. & / # ? %) must be url-escaped, not interpolated
// raw, when constructing the upstream search URL (T-16-03-01 mitigation).
func TestProvider_FindID_QuerySafetyEscape(t *testing.T) {
	t.Parallel()
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"total":0,"data":[]}`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)
	_, _ = p.FindID(context.Background(), domain.AnimeRef{Title: "a&b#c?d=e/f"})
	if capturedQuery == "" {
		t.Fatal("upstream never received the q= param")
	}
	if capturedQuery != "a&b#c?d=e/f" {
		t.Errorf("q param round-trip = %q; want \"a&b#c?d=e/f\"", capturedQuery)
	}
	// Crucially: the request RawQuery must contain `q=a%26b%23c%3Fd%3De%2Ff`
	// (or close — Go's url package may use + for spaces). We don't pin the
	// exact encoding here; we just verify the round-trip works, which it can
	// only do if we used url.QueryEscape (raw interpolation would yield
	// q=a&b#c?d=e/f → server would see q="a" and ignore the rest).
}

// TestNew_RequiresDependencies — WR-11 anchor. New now validates that every
// required Deps field is non-nil and returns an explicit error rather than
// silently constructing a Provider that nil-panics at first use.
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
		{
			name: "missing HTTP",
			deps: Deps{Embeds: reg, MalSync: fm, Cache: fc, Log: log},
			want: "HTTP",
		},
		{
			name: "missing Embeds",
			deps: Deps{HTTP: hc, MalSync: fm, Cache: fc, Log: log},
			want: "Embeds",
		},
		{
			name: "missing MalSync",
			deps: Deps{HTTP: hc, Embeds: reg, Cache: fc, Log: log},
			want: "MalSync",
		},
		{
			name: "missing Cache",
			deps: Deps{HTTP: hc, Embeds: reg, MalSync: fm, Log: log},
			want: "Cache",
		},
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

	// And the happy-path control: all four deps present → no error, Log may
	// be nil (falls back to logger.Default()).
	t.Run("optional log defaults", func(t *testing.T) {
		t.Parallel()
		p, err := New(Deps{HTTP: hc, Embeds: reg, MalSync: fm, Cache: fc})
		if err != nil {
			t.Fatalf("New(no log) = err %v; want nil", err)
		}
		if p == nil {
			t.Fatal("New(no log) returned nil Provider")
		}
	})
}

// TestProvider_ListEpisodes_ZeroMatchEmitsCounter — when upstream returns
// page 1 with empty `data` array, parser_zero_match_total{provider="animepahe",
// selector="episode_list_item"} increments by exactly 1. Anchors SCRAPER-NF-04.
//
// The test does NOT t.Parallel because the metric is a global counter; running
// in parallel with another test that also triggers it would flake the delta
// assertion. Other tests in this file use page-1 empty data fixtures but they
// each get a unique provider/selector child via the namespaced counter.
func TestProvider_ListEpisodes_ZeroMatchEmitsCounter(t *testing.T) {
	// Snapshot the counter BEFORE we trigger the emit so the test is robust
	// to whatever other tests have done to the global registry.
	before := testutil.ToFloat64(metrics.ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"current_page":1,"last_page":1,"data":[]}`))
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	eps, err := p.ListEpisodes(context.Background(), "12345-zero-match-test")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil (real-empty is not an error)", err)
	}
	if len(eps) != 0 {
		t.Fatalf("len(eps) = %d; want 0", len(eps))
	}

	after := testutil.ToFloat64(metrics.ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))
	if delta := after - before; delta != 1 {
		t.Errorf("parser_zero_match_total delta = %v; want 1", delta)
	}
}
