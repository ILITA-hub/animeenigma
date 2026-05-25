// orchestrator_phase18_test.go — Wave 0 integration test (Phase 18 Plan 18-04
// Task 1b) closing the gap flagged in 18-RESEARCH.md §Wave 0 Gaps + 18-CONTEXT.md S2.
//
// What this locks down end-to-end:
//
//  1. Two providers registered: a *health.FakeProvider configured as AnimePahe
//     (DOWN in the health cache) and a REAL *gogoanime.Provider wired against
//     an httptest.Server that serves the captured Plan 18-01 goldens.
//
//  2. The orchestrator's runFailover loop, when the cache reports AnimePahe
//     DOWN at the stream_segment stage, MUST skip every AnimePahe dispatch
//     and route FindID → ListEpisodes → ListServers → GetStream entirely
//     through gogoanime.
//
//  3. parser_fallback_total{from="animepahe", to="gogoanime"} MUST increment
//     for each skipped dispatch — at least once over the 4-call walk.
//
//  4. The skipped FakeProvider's GetStreamCalls() counter MUST remain 0
//     (the cache gate skips BEFORE method dispatch, not after).
//
// This is the unit-level proof of the same contract Task 5 (live smoke)
// validates against the running cluster — it lets us ship Phase 18 without
// waiting on a manual failover dance for every regression.
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime"
)

// stubMalSync is a malsync.moe stand-in that always returns miss. The 18-CONTEXT
// Open Q4 + RESEARCH note that malsync ships NO Gogoanime key as of 2026-05-12,
// so a miss-by-default stub exercises the fuzzy-search fallback path that is
// the actual production hot-path for Gogoanime.
type stubMalSync struct{}

func (s *stubMalSync) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	return "", false, nil
}

// memCache is an in-memory cache.Cache implementation. We can't reuse the
// gogoanime package's fakeCache (it's _test.go-scoped, not exported), so this
// is the smallest possible re-implementation that satisfies cache.Cache for
// the duration of this single integration test.
type memCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemCache() *memCache {
	return &memCache{data: make(map[string][]byte)}
}

func (m *memCache) Get(ctx context.Context, key string, dest interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; !ok {
		return cache.ErrNotFound
	}
	// We don't need real decoding here — every Get is a miss-by-design in this
	// test because we want the provider to walk the upstream pipeline end-to-end.
	return cache.ErrNotFound
}

func (m *memCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = []byte{}
	return nil
}

func (m *memCache) Delete(ctx context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *memCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *memCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	v, err := fn()
	if err != nil {
		return err
	}
	return m.Set(ctx, key, v, ttl)
}

func (m *memCache) Invalidate(ctx context.Context, pattern string) error { return nil }

func (m *memCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = []byte{}
	return true, nil
}

var _ cache.Cache = (*memCache)(nil)

// fakeEmbedExtractor is a wide-match extractor that matches every URL and
// returns a canned *Stream. It stands in for the real vibeplayer/streamhg/
// earnvids extractors so this test exercises the orchestrator + provider
// integration WITHOUT pulling in the embed extractors' own HTTP clients
// (which use their own http.DefaultTransport and can't be redirected from
// outside their package). The Phase 18 goldens-backed extractor coverage
// lives in services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go;
// here we only care that the orchestrator's failover path actually reaches a
// dispatchable extractor.
type fakeEmbedExtractor struct {
	streamURL string
}

func (f *fakeEmbedExtractor) Name() string         { return "fake-embed-phase18" }
func (f *fakeEmbedExtractor) Matches(string) bool  { return true }
func (f *fakeEmbedExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	return &domain.Stream{
		Sources: []domain.Source{{URL: f.streamURL, Type: "hls"}},
		Headers: map[string]string{"Referer": "https://anitaku.to/"},
	}, nil
}

var _ domain.EmbedExtractor = (*fakeEmbedExtractor)(nil)

// rewriteToSrvTransport routes every outgoing request to srvURL while keeping
// the original request scheme/host/path intact for the downstream handler
// (the handler routes on r.URL.Path, not Host). This mirrors the pattern in
// services/scraper/internal/embeds/packed_common_test.go but lives in this
// _test.go because the embeds version is package-private.
type rewriteToSrvTransport struct{ srvURL string }

func (r *rewriteToSrvTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(r.srvURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}

// loadFixture reads a Plan 18-01 golden fixture relative to the goldens dir.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return b
}

// TestOrchestrator_AnimePaheToGogoanimeFailover validates the Phase 18 Wave 0
// gap closure: when AnimePahe is reported DOWN in the in-memory health cache,
// the orchestrator MUST skip it for every business call (FindID, ListEpisodes,
// ListServers, GetStream) and dispatch to a REAL gogoanime.Provider that
// answers from offline goldens. parser_fallback_total must increment by at
// least 1.
//
// REVIEW (parallel-safety): provider names embed t.Name() to avoid collision
// with any other test that touches the same global parser_fallback_total
// counter. The pattern is borrowed from TestOrchestrator_SkipsUnhealthyProvider.
func TestOrchestrator_AnimePaheToGogoanimeFailover(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Load every golden up-front so a missing fixture fails the test fast
	//    rather than mid-way through the dispatch chain.
	//
	//    The category + episode goldens are One Piece, so the search input must
	//    resolve to the "one-piece" slug too. The previously-paired
	//    search_attack_on_titan.html golden made FindID resolve an
	//    "attack-on-titan-*" slug, which fetchEpisodes' slug-gate (added to
	//    filter the "Recent Releases" sidebar) then correctly stripped from the
	//    One Piece episode list → 0 episodes. We use a minimal SYNTHETIC search
	//    fragment here (only the `p.name a[href^='/category/']` structure FindID
	//    parses matters) so the whole search→category→episode chain is One Piece.
	searchHTML := []byte(`<!doctype html><html><body><ul class="items">` +
		`<li><p class="name"><a href="/category/one-piece" title="One Piece">One Piece</a></p></li>` +
		`</ul></body></html>`)
	categoryHTML := loadFixture(t, "category_one_piece.html")
	episodeHTML := loadFixture(t, "one_piece_episode_1.html")

	// 2. httptest.Server that routes by path-prefix to the right golden.
	//    The Provider walks /search.html -> /category/<slug> -> /<epID>, so a
	//    three-way switch on r.URL.Path is sufficient.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search.html"):
			_, _ = w.Write(searchHTML)
		case strings.HasPrefix(r.URL.Path, "/category/"):
			// One Piece dub page would also be hit (ListEpisodes merges sub+dub).
			// Serving the same sub golden for both is fine: the provider routes
			// on the slug, not the body, and we only need a valid response.
			_, _ = w.Write(categoryHTML)
		case strings.Contains(r.URL.Path, "-episode-"):
			_, _ = w.Write(episodeHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// 3. BaseHTTPClient wired with WithTransport so every outgoing fetch from
	//    gogoanime.Provider lands at the test server. Retries off — fail fast.
	log := logger.Default()
	baseHTTP := domain.NewBaseHTTPClient(
		log,
		domain.WithTransport(&rewriteToSrvTransport{srvURL: srv.URL}),
		domain.WithMaxRetries(0),
	)

	// 4. embeds.Registry holds the fake extractor — matches every URL.
	registry := domain.NewRegistry()
	registry.Register(&fakeEmbedExtractor{streamURL: "https://stream.example.test/playlist.m3u8"})

	// 5. Real gogoanime.Provider wired against the test server, in-memory
	//    cache, malsync-always-miss stub.
	gogo, err := gogoanime.New(gogoanime.Deps{
		BaseURL: srv.URL,
		HTTP:    baseHTTP,
		Embeds:  registry,
		MalSync: &stubMalSync{},
		Cache:   newMemCache(),
		Log:     log,
	})
	if err != nil {
		t.Fatalf("gogoanime.New: %v", err)
	}

	// 6. Fake AnimePahe — DOWN in the cache + every business method records a
	//    test failure if invoked (the cache gate should skip BEFORE dispatch).
	fakePaheName := "animepahe_failover_" + t.Name()
	gogoName := "gogoanime"

	fakePahe := &health.FakeProvider{
		NameVal: fakePaheName,
		FindIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			t.Errorf("%s.FindID was called; expected skip via cache DOWN", fakePaheName)
			return "", nil
		},
		ListEpisodesFn: func(ctx context.Context, providerID string) ([]domain.Episode, error) {
			t.Errorf("%s.ListEpisodes was called; expected skip via cache DOWN", fakePaheName)
			return nil, nil
		},
		ListServersFn: func(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
			t.Errorf("%s.ListServers was called; expected skip via cache DOWN", fakePaheName)
			return nil, nil
		},
		GetStreamFn: func(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category) (*domain.Stream, error) {
			t.Errorf("%s.GetStream was called; expected skip via cache DOWN", fakePaheName)
			return nil, nil
		},
	}

	// 7. Health cache: mark the fake AnimePahe DOWN at stream_segment (the
	//    canonical stage IsHealthy() checks; see internal/health/cache.go).
	healthCache := health.NewInMemoryHealthCache()
	healthCache.Update(fakePaheName, health.ProviderHealth{
		Stages:      map[string]health.StageStatus{health.StageStreamSegment: {Up: false}},
		LastUpdated: time.Now(),
	})

	// 8. Orchestrator: register the fake AnimePahe FIRST and the real gogoanime
	//    SECOND — registration order is failover order (CONTEXT.md D5). The
	//    health cache gate causes every call to skip the first provider.
	orch := NewOrchestrator(log, domain.NewRegistry(), healthCache)
	orch.Register(fakePahe)
	orch.Register(gogo)

	// 9. Snapshot the failover counter before the walk.
	before := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues(fakePaheName, gogoName))

	// 10. Walk FindID → ListEpisodes → ListServers → GetStream end-to-end.
	ref := domain.AnimeRef{Title: "One Piece", ShikimoriID: "21"}
	animeID, err := orch.FindID(ctx, ref, "")
	if err != nil {
		t.Fatalf("orch.FindID: %v", err)
	}
	if animeID == "" {
		t.Fatal("orch.FindID returned empty slug")
	}

	episodes, err := orch.ListEpisodes(ctx, animeID, "")
	if err != nil {
		t.Fatalf("orch.ListEpisodes: %v", err)
	}
	if len(episodes) == 0 {
		t.Fatal("orch.ListEpisodes returned 0 episodes")
	}

	servers, err := orch.ListServers(ctx, animeID, episodes[0].ID, "")
	if err != nil {
		t.Fatalf("orch.ListServers: %v", err)
	}
	if len(servers) == 0 {
		t.Fatal("orch.ListServers returned 0 servers")
	}

	stream, err := orch.GetStream(ctx, animeID, episodes[0].ID, servers[0].ID, domain.CategorySub, "")
	if err != nil {
		t.Fatalf("orch.GetStream: %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatal("orch.GetStream returned empty stream")
	}

	// 11. parser_fallback_total{from=fakePahe,to=gogoanime} must have
	//     incremented at least once across the 4-call walk. The cache gate
	//     emits the metric per skip (see orchestrator.go:206).
	after := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues(fakePaheName, gogoName))
	if d := after - before; d < 1.0 {
		t.Errorf("parser_fallback_total{from=%s,to=%s} delta = %v; want >= 1", fakePaheName, gogoName, d)
	}

	// 12. The skipped FakeProvider's GetStreamCalls() MUST remain 0 — the
	//     cache gate skips BEFORE dispatch, not after.
	if c := fakePahe.GetStreamCalls(); c != 0 {
		t.Errorf("fakePahe.GetStreamCalls() = %d; want 0 (cache-gate skip is broken)", c)
	}
	if c := fakePahe.FindIDCalls(); c != 0 {
		t.Errorf("fakePahe.FindIDCalls() = %d; want 0 (cache-gate skip is broken)", c)
	}
	if c := fakePahe.ListEpisodesCalls(); c != 0 {
		t.Errorf("fakePahe.ListEpisodesCalls() = %d; want 0 (cache-gate skip is broken)", c)
	}
	if c := fakePahe.ListServersCalls(); c != 0 {
		t.Errorf("fakePahe.ListServersCalls() = %d; want 0 (cache-gate skip is broken)", c)
	}
}
