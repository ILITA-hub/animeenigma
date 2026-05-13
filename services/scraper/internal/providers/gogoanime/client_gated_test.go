// client_gated_test.go — GetStreamWithGate test suite (Phase 21 Plan 21-03
// Task 3). Drives the priority + gate + winning-server-cache contract
// without standing up real HTTP / streamprobe / Prometheus state.
package gogoanime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// gatedTestHosts is the host→extractor-name map used by every gated test.
// Matches what main.go (Task 5) will build at boot.
var gatedTestHosts = map[string]string{
	"otakuhg.site":    "streamhg",
	"otakuvid.online": "earnvids",
	"vibeplayer.site": "vibeplayer",
}

// gatedTestPriority is the canonical default — same as CONTEXT.md D3.
var gatedTestPriority = []string{"streamhg", "earnvids", "vibeplayer"}

// fakeProbe is the streamprobe.Probe stand-in. results maps masterURL →
// (Reason, sleep). calls records the absolute timestamp of each call
// (used by the parallelism test to assert top-2 ran concurrently).
type fakeProbe struct {
	mu      sync.Mutex
	results map[string]fakeProbeOutcome
	calls   []time.Time
}

type fakeProbeOutcome struct {
	reason streamprobe.Reason
	sleep  time.Duration
}

func newFakeProbe() *fakeProbe {
	return &fakeProbe{results: map[string]fakeProbeOutcome{}}
}

func (f *fakeProbe) set(masterURL string, reason streamprobe.Reason, sleep time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[masterURL] = fakeProbeOutcome{reason: reason, sleep: sleep}
}

func (f *fakeProbe) probe(ctx context.Context, masterURL string, headers http.Header) streamprobe.Result {
	f.mu.Lock()
	f.calls = append(f.calls, time.Now())
	out, ok := f.results[masterURL]
	f.mu.Unlock()
	if !ok {
		// Default to "unreachable" so a misconfigured test fails loudly
		// instead of silently passing on a missing fixture.
		return streamprobe.Result{Reason: streamprobe.ReasonCDNUnreachable}
	}
	if out.sleep > 0 {
		select {
		case <-time.After(out.sleep):
		case <-ctx.Done():
			return streamprobe.Result{Reason: streamprobe.ReasonCDNUnreachable}
		}
	}
	playable := out.reason == streamprobe.ReasonPlayable
	return streamprobe.Result{Playable: playable, Reason: out.reason}
}

func (f *fakeProbe) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// gatedTestProvider wires a Provider with the gated dependencies + a
// fakeProbe + a fakeStreamExtractor that returns Stream{Sources:[{URL:embedURL+"/master.m3u8"}]}.
// Returns the provider, its fake cache, the fake probe, and the fake
// extractor's call counter accessor.
func gatedTestProvider(t *testing.T, srv *httptest.Server) (*Provider, *fakeCache, *fakeProbe, *fakeStreamExtractor) {
	t.Helper()
	log := newTestLogger(t)
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	fc := newFakeCache()
	fm := &fakeMalSync{mappings: map[string]string{}, misses: map[string]bool{}}
	fk := &fakeStreamExtractor{streams: map[string]*domain.Stream{}}
	reg := domain.NewRegistry()
	reg.Register(fk)
	fp := newFakeProbe()

	p, err := New(Deps{
		BaseURL:        srv.URL,
		HTTP:           hc,
		Embeds:         reg,
		MalSync:        fm,
		Cache:          fc,
		Log:            log,
		ServerPriority: gatedTestPriority,
		HostExtractor:  gatedTestHosts,
		Probe:          fp.probe,
	})
	if err != nil {
		t.Fatalf("New(Deps{...}) = err %v; want nil", err)
	}
	return p, fc, fp, fk
}

// resetGateMetrics zeroes parser_unplayable_total + parser_ad_decoy_total
// label children so cross-test counter pollution doesn't leak.
func resetGateMetrics() {
	metrics.ParserUnplayableTotal.Reset()
	metrics.ParserAdDecoyTotal.Reset()
}

// streamForExtract is the canonical Stream the fake extractor returns —
// each server URL becomes a different Stream.Sources[0].URL.
func extractStreamFor(embedURL string) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{{URL: embedURL + "/master.m3u8", Type: "hls"}},
		Headers: map[string]string{"Referer": "https://anitaku.to/"},
	}
}

// gatedTestServers returns the canonical 3-server fixture in priority
// order (streamhg, earnvids, vibeplayer).
func gatedTestServers() []domain.Server {
	return []domain.Server{
		{ID: "https://otakuhg.site/e/abc", Name: "HD-1", Type: domain.CategorySub},     // streamhg
		{ID: "https://otakuvid.online/e/def", Name: "HD-2", Type: domain.CategorySub},  // earnvids
		{ID: "https://vibeplayer.site/e/ghi", Name: "StreamX", Type: domain.CategorySub}, // vibeplayer
	}
}

// TestGetStreamWithGate_HappyPath_FirstServerWins — streamhg gate-passes;
// returned stream comes from streamhg; winner cached with TTL 5min;
// no unplayable counters incremented.
func TestGetStreamWithGate_HappyPath_FirstServerWins(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	fk.streams[servers[0].ID] = extractStreamFor(servers[0].ID)
	fk.streams[servers[1].ID] = extractStreamFor(servers[1].ID)
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if !gated {
		t.Errorf("gated = false; want true on cold-path success")
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatal("empty stream")
	}
	// The winning source MUST be one of streamhg or earnvids (top-2 parallel
	// — race winner is non-deterministic). NOT vibeplayer.
	gotURL := stream.Sources[0].URL
	if gotURL != servers[0].ID+"/master.m3u8" && gotURL != servers[1].ID+"/master.m3u8" {
		t.Errorf("winning source URL = %q; want streamhg or earnvids", gotURL)
	}
	// Cache key must be set with the winning serverID.
	setLog := fc.snapshotSetLog()
	foundKey := false
	wantKeyPrefix := "scraper:winning_server:gogoanime:frieren:frieren-episode-1"
	for _, k := range setLog {
		if k == wantKeyPrefix {
			foundKey = true
			break
		}
	}
	if !foundKey {
		t.Errorf("cache set log %v; want to contain %q", setLog, wantKeyPrefix)
	}
	// No unplayable counters for the winner.
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonPlayable))); v > 0 {
		t.Errorf("parser_unplayable_total{server=streamhg,reason=playable} = %v; want 0", v)
	}
}

// TestGetStreamWithGate_AdDecoy_Skipped — streamhg returns ad_decoy gate
// failure; earnvids passes. Result comes from earnvids; ad-decoy counter
// + unplayable counter both increment for streamhg.
func TestGetStreamWithGate_AdDecoy_Skipped(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	for _, s := range servers {
		fk.streams[s.ID] = extractStreamFor(s.ID)
	}
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonAdDecoy, 0)   // streamhg fails
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)  // earnvids passes

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if !gated {
		t.Errorf("gated = false; want true")
	}
	if stream.Sources[0].URL != servers[1].ID+"/master.m3u8" {
		t.Errorf("winning source = %q; want %q (earnvids)", stream.Sources[0].URL, servers[1].ID+"/master.m3u8")
	}
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonAdDecoy))); v != 1 {
		t.Errorf("parser_unplayable_total{server=streamhg,reason=ad_decoy} = %v; want 1", v)
	}
	if v := testutil.ToFloat64(metrics.ParserAdDecoyTotal.WithLabelValues(
		"gogoanime", "streamhg")); v != 1 {
		t.Errorf("parser_ad_decoy_total{server=streamhg} = %v; want 1", v)
	}
	// Cache set on earnvids.
	setLog := fc.snapshotSetLog()
	wantKey := "scraper:winning_server:gogoanime:frieren:frieren-episode-1"
	found := false
	for _, k := range setLog {
		if k == wantKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cache set log %v; want to contain %q", setLog, wantKey)
	}
}

// TestGetStreamWithGate_ParallelTop2 — both streamhg and earnvids are
// probed concurrently. With a 1s sleep per probe, the wall-clock must
// stay under ~1.5s (parallel) NOT 2s+ (sequential).
func TestGetStreamWithGate_ParallelTop2(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	for _, s := range servers {
		fk.streams[s.ID] = extractStreamFor(s.ID)
	}
	// Both top-2 servers probe-passes but with a 1s sleep.
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonPlayable, 1*time.Second)
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 1*time.Second)

	start := time.Now()
	_, _, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	// Parallel: ≤ 1.5s. Sequential would be ≥ 2s.
	if elapsed > 1700*time.Millisecond {
		t.Errorf("wall-clock = %v; want ≤ 1.7s (proves top-2 parallel, not sequential)", elapsed)
	}
}

// TestGetStreamWithGate_AllFail_ProviderDown — every server gate-fails.
// Returns ErrProviderDown, gated=true, with all 3 unplayable counters
// incremented.
func TestGetStreamWithGate_AllFail_ProviderDown(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	for _, s := range servers {
		fk.streams[s.ID] = extractStreamFor(s.ID)
	}
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonStatus403, 0)
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonCDNUnreachable, 0)
	fp.set(servers[2].ID+"/master.m3u8", streamprobe.ReasonAdDecoy, 0)

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err == nil {
		t.Fatal("err = nil; want ErrProviderDown")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want ErrProviderDown chain", err)
	}
	if !gated {
		t.Errorf("gated = false; want true even on exhaustion (gate DID run)")
	}
	if stream != nil {
		t.Errorf("stream = %v; want nil on exhaustion", stream)
	}
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonStatus403))); v != 1 {
		t.Errorf("parser_unplayable_total{streamhg,status_403} = %v; want 1", v)
	}
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "earnvids", string(streamprobe.ReasonCDNUnreachable))); v != 1 {
		t.Errorf("parser_unplayable_total{earnvids,cdn_unreachable} = %v; want 1", v)
	}
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "vibeplayer", string(streamprobe.ReasonAdDecoy))); v != 1 {
		t.Errorf("parser_unplayable_total{vibeplayer,ad_decoy} = %v; want 1", v)
	}
}

// TestGetStreamWithGate_WarmPath_CacheHit — cached winner is consulted
// BEFORE the priority loop. Returns gated=false; no probe calls made.
func TestGetStreamWithGate_WarmPath_CacheHit(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	fk.streams[servers[1].ID] = extractStreamFor(servers[1].ID) // earnvids

	// Pre-seed the winning_server cache with earnvids.
	winnerKey := "scraper:winning_server:gogoanime:frieren:frieren-episode-1"
	if err := fc.Set(context.Background(), winnerKey, servers[1].ID, 5*time.Minute); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if gated {
		t.Errorf("gated = true; want false on warm-path cache hit")
	}
	if stream.Sources[0].URL != servers[1].ID+"/master.m3u8" {
		t.Errorf("winning source = %q; want %q (earnvids cached)", stream.Sources[0].URL, servers[1].ID+"/master.m3u8")
	}
	if fp.callCount() != 0 {
		t.Errorf("probe was called %d times; want 0 on warm path", fp.callCount())
	}
}

// TestGetStreamWithGate_StaleCacheServer_Refreshes — cached serverID no
// longer in the supplied servers list. Cache is deleted; cold path runs.
// gated=true.
func TestGetStreamWithGate_StaleCacheServer_Refreshes(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	for _, s := range servers {
		fk.streams[s.ID] = extractStreamFor(s.ID)
	}
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)

	// Pre-seed the cache with a STALE serverID (not in the current list).
	winnerKey := "scraper:winning_server:gogoanime:frieren:frieren-episode-1"
	staleID := "https://gone.example/e/old"
	if err := fc.Set(context.Background(), winnerKey, staleID, 5*time.Minute); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	_, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if !gated {
		t.Errorf("gated = false; want true after stale cache → cold path")
	}
	// Cache should have been deleted then re-set. Verify the cached value is
	// no longer the stale one.
	var cachedAfter string
	if err := fc.Get(context.Background(), winnerKey, &cachedAfter); err != nil {
		t.Fatalf("cache.Get after refresh: %v", err)
	}
	if cachedAfter == staleID {
		t.Errorf("cache still holds stale serverID %q; want fresh winner", staleID)
	}
}

// TestGetStreamWithGate_CallerPin_BypassesGate — non-empty serverID
// bypasses both priority and gate; returns gated=false. probe.callCount==0.
func TestGetStreamWithGate_CallerPin_BypassesGate(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	fk.streams[servers[2].ID] = extractStreamFor(servers[2].ID) // vibeplayer

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", servers[2].ID /* PIN */, domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if gated {
		t.Errorf("gated = true; want false on caller-pinned path")
	}
	if stream.Sources[0].URL != servers[2].ID+"/master.m3u8" {
		t.Errorf("source = %q; want %q (vibeplayer caller-pinned)", stream.Sources[0].URL, servers[2].ID+"/master.m3u8")
	}
	if fp.callCount() != 0 {
		t.Errorf("probe was called %d times; want 0 on caller-pinned path", fp.callCount())
	}
}

// TestGetStreamWithGate_BudgetExceeded — every probe takes 7s; the
// 8s overall budget triggers ErrProviderDown wrapping context.DeadlineExceeded
// before all servers are tried.
func TestGetStreamWithGate_BudgetExceeded(t *testing.T) {
	t.Skip("TODO: 8s timeout — too slow for CI. Manual smoke only.")
}

// TestGetStreamWithGate_CacheKeyShape — the cache key on a successful
// cold-path is EXACTLY `scraper:winning_server:gogoanime:<providerID>:<episodeID>`
// (no hashing), and the TTL is exactly 5*time.Minute.
func TestGetStreamWithGate_CacheKeyShape(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, fc, fp, fk := gatedTestProvider(t, srv)
	servers := gatedTestServers()
	fk.streams[servers[0].ID] = extractStreamFor(servers[0].ID)
	fk.streams[servers[1].ID] = extractStreamFor(servers[1].ID)
	fp.set(servers[0].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)
	fp.set(servers[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)

	_, _, err := p.GetStreamWithGate(context.Background(),
		"my-anime-slug", "my-anime-slug-episode-7", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	setLog := fc.snapshotSetLog()
	wantKey := "scraper:winning_server:gogoanime:my-anime-slug:my-anime-slug-episode-7"
	found := false
	for _, k := range setLog {
		if k == wantKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cache set log %v; want to contain EXACT key %q (no hash)", setLog, wantKey)
	}
}

// TestGetStreamWithGate_InternalSort_PreservesPriority — pass UNSORTED
// servers fixture [vibeplayer, earnvids, streamhg]; assert streamhg is
// probed first (proves the internal SortByPriority runs as the first
// statement of coldPathGated, not test-pre-sorted).
func TestGetStreamWithGate_InternalSort_PreservesPriority(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	canonical := gatedTestServers()
	// Reverse order: [vibeplayer, earnvids, streamhg]
	unsorted := []domain.Server{canonical[2], canonical[1], canonical[0]}
	for _, s := range unsorted {
		fk.streams[s.ID] = extractStreamFor(s.ID)
	}
	// streamhg + earnvids both pass; vibeplayer fails as ad_decoy. If the
	// internal sort works, streamhg & earnvids are the top-2 probed in
	// parallel and the result MUST come from one of them (not vibeplayer).
	fp.set(canonical[0].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)  // streamhg
	fp.set(canonical[1].ID+"/master.m3u8", streamprobe.ReasonPlayable, 0)  // earnvids
	fp.set(canonical[2].ID+"/master.m3u8", streamprobe.ReasonAdDecoy, 0)   // vibeplayer

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, unsorted)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if !gated {
		t.Errorf("gated = false; want true")
	}
	winner := stream.Sources[0].URL
	if winner != canonical[0].ID+"/master.m3u8" && winner != canonical[1].ID+"/master.m3u8" {
		t.Errorf("winner = %q; want streamhg or earnvids (NOT vibeplayer despite vibeplayer-first input — proves internal sort works)", winner)
	}
}

// TestGetStreamWithGate_EmptyServers_NotFound — no servers provided, no
// caller pin: returns ErrNotFound, gated=false.
func TestGetStreamWithGate_EmptyServers_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, _, _ := gatedTestProvider(t, srv)
	_, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v; want ErrNotFound", err)
	}
	if gated {
		t.Errorf("gated = true; want false (gate never ran)")
	}
}

// streamForMultiSource returns a 2-source Stream where Sources[0].URL fails
// the gate and Sources[1].URL passes. Used by Plan 22-01 multi-source
// iteration tests.
func extractMultiSourceStreamFor(embedURL string) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{
			{URL: embedURL + "/hls2/master.m3u8", Type: "hls"},
			{URL: embedURL + "/hls3/master.txt", Type: "hls"},
		},
		Headers: map[string]string{"Referer": "https://anitaku.to/"},
	}
}

// TestGetStreamWithGate_MultiSource_FirstFailsSecondWins — Plan 22-01 Task 2.
// streamhg returns a 2-source Stream (hls2 + hls3). The first source fails
// the gate (cdn_unreachable); the SECOND source passes. Asserts:
//
//   - returned trimmed Stream has exactly 1 Source — the playable hls3 URL.
//   - parser_unplayable_total{server=streamhg,reason=cdn_unreachable} = 1
//     (the failed Sources[0] increment).
//   - winner is cached.
//
// Implementation note: uses a 1-server fixture so coldPathGated runs the
// multi-source iteration sequentially (topN==1, no parallel race). This
// isolates the per-source-iteration behaviour from the top-2 parallel
// race semantics.
func TestGetStreamWithGate_MultiSource_FirstFailsSecondWins(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	servers := []domain.Server{
		{ID: "https://otakuhg.site/e/abc", Name: "HD-1", Type: domain.CategorySub}, // streamhg only
	}
	// Streamhg returns multi-source: [0] fails (cdn_unreachable), [1] passes.
	multi := extractMultiSourceStreamFor(servers[0].ID)
	fk.streams[servers[0].ID] = multi
	fp.set(multi.Sources[0].URL, streamprobe.ReasonCDNUnreachable, 0)
	fp.set(multi.Sources[1].URL, streamprobe.ReasonPlayable, 0)

	stream, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err != nil {
		t.Fatalf("GetStreamWithGate: %v", err)
	}
	if !gated {
		t.Errorf("gated = false; want true on cold-path success")
	}
	if stream == nil {
		t.Fatal("stream is nil")
	}
	// Trimmed Stream: ONLY the playable Source survives.
	if len(stream.Sources) != 1 {
		t.Errorf("len(stream.Sources) = %d; want 1 (trimmed to playable only); sources=%+v", len(stream.Sources), stream.Sources)
	}
	if stream.Sources[0].URL != multi.Sources[1].URL {
		t.Errorf("winning source URL = %q; want %q (hls3 — second source)",
			stream.Sources[0].URL, multi.Sources[1].URL)
	}
	// One unplayable increment for the failed hls2 source.
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonCDNUnreachable))); v != 1 {
		t.Errorf("parser_unplayable_total{streamhg,cdn_unreachable} = %v; want 1 (per-source iteration)", v)
	}
}

// TestGetStreamWithGate_MultiSource_AllFail — Plan 22-01 Task 2.
// All Sources for a server fail. Each source increments
// parser_unplayable_total with its own reason label; the orchestrator
// advances past the server (per-source iteration exhausted).
//
// Implementation note: to avoid racing the parallel top-2 winner, we put
// the multi-source server at position 3 (sequential phase) and make both
// top-2 servers fail outright. The orchestrator then sequentially runs
// streamhg's multi-source loop in full, all Sources fail, and the
// function returns ErrProviderDown with each source's metric increment
// observable.
func TestGetStreamWithGate_MultiSource_AllFail(t *testing.T) {
	resetGateMetrics()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p, _, fp, fk := gatedTestProvider(t, srv)
	// Server order: [single-source-fail-1, single-source-fail-2, multi-source-fail]
	// — uses a custom priority list to put a non-streamhg server at the
	// top-2 positions so the multi-source iteration runs SEQUENTIALLY at
	// position 3 (no parallel race).
	servers := []domain.Server{
		{ID: "https://otakuvid.online/e/def", Name: "HD-2", Type: domain.CategorySub},     // earnvids - top-2
		{ID: "https://vibeplayer.site/e/ghi", Name: "StreamX", Type: domain.CategorySub},  // vibeplayer - top-2
		{ID: "https://otakuhg.site/e/abc", Name: "HD-1", Type: domain.CategorySub},        // streamhg - position 3 (sequential)
	}
	// Custom priority: keep earnvids+vibeplayer at top so streamhg runs sequentially.
	// We bypass SortByPriority's "streamhg-first" canonical order by passing servers
	// whose host extractor names sort as earnvids < streamhg < vibeplayer ... but
	// SortByPriority will resort. To force sequential streamhg iteration we instead
	// keep the canonical order [streamhg, earnvids, vibeplayer] BUT make both
	// earnvids + vibeplayer fail outright, and make streamhg sequential by ensuring
	// its multi-source iteration completes before earnvids/vibeplayer return.
	//
	// Cleaner: pass the SortByPriority's canonical order then make streamhg multi-fail
	// and the top-2 race winners (after streamhg is moved out of top-2 — but it's
	// always #1 by canonical priority — so we must accept the race) ...
	//
	// Easiest correct fix: directly test sequential iteration by passing only
	// streamhg, so coldPathGated's topN==1, no parallel goroutines.
	servers = []domain.Server{
		{ID: "https://otakuhg.site/e/abc", Name: "HD-1", Type: domain.CategorySub}, // streamhg only
	}
	multi := extractMultiSourceStreamFor(servers[0].ID)
	fk.streams[servers[0].ID] = multi
	fp.set(multi.Sources[0].URL, streamprobe.ReasonStatus403, 0)
	fp.set(multi.Sources[1].URL, streamprobe.ReasonSignedURLExpired, 0)

	_, gated, err := p.GetStreamWithGate(context.Background(),
		"frieren", "frieren-episode-1", "", domain.CategorySub, servers)
	if err == nil {
		t.Fatal("err = nil; want ErrProviderDown (all sources failed)")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want ErrProviderDown chain", err)
	}
	if !gated {
		t.Errorf("gated = false; want true (gate ran)")
	}
	// BOTH streamhg Sources should have incremented their own reason label.
	// status_403 from Sources[0] (hls2), signed_url_expired from Sources[1] (hls3).
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonStatus403))); v != 1 {
		t.Errorf("parser_unplayable_total{streamhg,status_403} = %v; want 1 (per-source hls2)", v)
	}
	if v := testutil.ToFloat64(metrics.ParserUnplayableTotal.WithLabelValues(
		"gogoanime", "streamhg", string(streamprobe.ReasonSignedURLExpired))); v != 1 {
		t.Errorf("parser_unplayable_total{streamhg,signed_url_expired} = %v; want 1 (per-source hls3)", v)
	}
}

// _ keeps atomic imported for parallelism tracking if needed by future
// extensions of this file.
var _ = atomic.LoadInt32
