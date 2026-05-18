package health

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
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

// newTestLogger builds a minimal logger that doesn't spam tests.
func newProbeTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// freshClock returns a closure that yields advancing virtual time. Each
// call to `now()` returns the current value; advance via `advance(d)`.
func freshClock(t0 time.Time) (now func() time.Time, advance func(time.Duration)) {
	var mu sync.Mutex
	cur := t0
	now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	advance = func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		cur = cur.Add(d)
	}
	return
}

// successfulStream returns a *domain.Stream with one playable source
// pointing at the given URL.
func successfulStream(sourceURL string) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{{URL: sourceURL, Type: "hls"}},
	}
}

// resetProviderMetrics zeros the global metric children for `name` so each
// test starts from a known state. Avoids cross-test bleed.
func resetProviderMetrics(name string) {
	for _, s := range AllStages {
		metrics.ProviderHealthUp.DeleteLabelValues(name, s)
	}
	metrics.ProviderProbeLastTick.DeleteLabelValues(name)
}

// fullSuccessProvider builds a FakeProvider whose every method returns a
// healthy result. Stream sources point at `streamURL` (typically an httptest
// server).
func fullSuccessProvider(name, streamURL string) *FakeProvider {
	return &FakeProvider{
		NameVal:        name,
		FindIDFn:       func(_ context.Context, _ domain.AnimeRef) (string, error) { return "anime-id", nil },
		ListEpisodesFn: func(_ context.Context, _ string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "ep-1", Number: 1}}, nil
		},
		ListServersFn: func(_ context.Context, _, _ string) ([]domain.Server, error) {
			return []domain.Server{{ID: "server-1", Name: "kwik", Type: domain.CategorySub}}, nil
		},
		GetStreamFn: func(_ context.Context, _, _, _ string, _ domain.Category) (*domain.Stream, error) {
			return successfulStream(streamURL), nil
		},
	}
}

// TestProbe_ThreeConsecutiveFailures_FlipsGaugeDown — drive 3 FindID
// failures at virtual times spread within 15 min; assert search gauge = 0.
func TestProbe_ThreeConsecutiveFailures_FlipsGaugeDown(t *testing.T) {
	name := "tp-three-fails"
	defer resetProviderMetrics(name)

	fake := &FakeProvider{
		NameVal: name,
		FindIDFn: func(_ context.Context, _ domain.AnimeRef) (string, error) {
			return "", errors.New("boom")
		},
	}
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, advance := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)

	r.RunOnce(context.Background())
	advance(5 * time.Minute)
	r.RunOnce(context.Background())
	advance(5 * time.Minute)
	r.RunOnce(context.Background())

	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 0 {
		t.Errorf("provider_health_up{stage=search} = %v; want 0", got)
	}
}

// TestProbe_FirstSuccessAfterDown_FlipsBackUp — flip down, then return
// nil on FindID + full success thereafter → gauge flips to 1.
func TestProbe_FirstSuccessAfterDown_FlipsBackUp(t *testing.T) {
	name := "tp-recover"
	defer resetProviderMetrics(name)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 8192))
	}))
	defer srv.Close()

	var callCount atomic.Int32
	fake := fullSuccessProvider(name, srv.URL)
	originalFindID := fake.FindIDFn
	fake.FindIDFn = func(ctx context.Context, ref domain.AnimeRef) (string, error) {
		n := callCount.Add(1)
		if n <= 3 {
			return "", errors.New("transient")
		}
		return originalFindID(ctx, ref)
	}

	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, advance := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)
	allowPrivateHostsForTest(r) // BLK-01: httptest binds 127.0.0.1 (WR-NEW-02: test-only helper)

	// 3 failures → down
	for i := 0; i < 3; i++ {
		r.RunOnce(context.Background())
		advance(5 * time.Minute)
	}
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 0 {
		t.Fatalf("setup: provider_health_up{stage=search} = %v; want 0", got)
	}

	// 4th tick succeeds → flip back to up
	r.RunOnce(context.Background())
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 1 {
		t.Errorf("after recovery, provider_health_up{stage=search} = %v; want 1", got)
	}
	snap := cache.AdminSnapshot()
	if h, ok := snap[name]; !ok || h.Stages[StageSearch].LastOK.IsZero() {
		t.Errorf("expected LastOK to be set in cache for stage=search; got %+v", snap[name])
	}
}

// TestProbe_StaleFailuresOutsideWindow_StayUp — failures at t, t+8, t+17:
// after the prune at t+17 only t+8 + t+17 remain in the window → gauge
// stays 1.
func TestProbe_StaleFailuresOutsideWindow_StayUp(t *testing.T) {
	name := "tp-stale-prune"
	defer resetProviderMetrics(name)

	fake := &FakeProvider{
		NameVal: name,
		FindIDFn: func(_ context.Context, _ domain.AnimeRef) (string, error) {
			return "", errors.New("blip")
		},
	}
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, advance := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)

	r.RunOnce(context.Background())
	advance(8 * time.Minute)
	r.RunOnce(context.Background())
	advance(9 * time.Minute) // now at t+17
	r.RunOnce(context.Background())

	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 1 {
		t.Errorf("stale-failure prune: provider_health_up{stage=search} = %v; want 1", got)
	}
}

// TestProbe_LastTickHeartbeatAdvances — after each tick, the heartbeat
// gauge equals now.Unix().
func TestProbe_LastTickHeartbeatAdvances(t *testing.T) {
	name := "tp-heartbeat"
	defer resetProviderMetrics(name)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 8192))
	}))
	defer srv.Close()
	fake := fullSuccessProvider(name, srv.URL)
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, advance := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)
	allowPrivateHostsForTest(r) // BLK-01: httptest binds 127.0.0.1 (WR-NEW-02: test-only helper)

	r.RunOnce(context.Background())
	if got := testutil.ToFloat64(metrics.ProviderProbeLastTick.WithLabelValues(name)); got != float64(t0.Unix()) {
		t.Errorf("heartbeat after tick 1 = %v; want %v", got, t0.Unix())
	}
	advance(2 * time.Minute)
	r.RunOnce(context.Background())
	if got := testutil.ToFloat64(metrics.ProviderProbeLastTick.WithLabelValues(name)); got != float64(nowFn().Unix()) {
		t.Errorf("heartbeat after tick 2 = %v; want %v", got, nowFn().Unix())
	}
}

// TestProbe_PanicInProviderRecovers — provider panics on first call but
// returns success on second. runOneTickSafely contains the panic; the next
// tick proceeds normally.
func TestProbe_PanicInProviderRecovers(t *testing.T) {
	name := "tp-panic"
	defer resetProviderMetrics(name)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 8192))
	}))
	defer srv.Close()

	var callCount atomic.Int32
	fake := fullSuccessProvider(name, srv.URL)
	originalFindID := fake.FindIDFn
	fake.FindIDFn = func(ctx context.Context, ref domain.AnimeRef) (string, error) {
		if callCount.Add(1) == 1 {
			panic("simulated provider explosion")
		}
		return originalFindID(ctx, ref)
	}

	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, advance := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)
	allowPrivateHostsForTest(r) // BLK-01: httptest binds 127.0.0.1 (WR-NEW-02: test-only helper)

	// runOneTickSafely MUST not propagate the panic.
	r.runOneTickSafely(context.Background())
	advance(1 * time.Minute)

	// Second tick should succeed normally.
	r.runOneTickSafely(context.Background())
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 1 {
		t.Errorf("after recovery, provider_health_up{stage=search} = %v; want 1", got)
	}
}

// TestProbe_StagesShortCircuitOnFirstFailure — FindID fails → ListEpisodes,
// ListServers, GetStream are NOT called (short-circuit preserves upstream
// load). The cache and gauges for downstream stages are flipped to Up=false
// with an explanatory LastErr so the health endpoint reports honest state
// instead of carrying stale-OK from a previous tick. The window state is
// NOT touched — these are "uncheckable", not real failures.
func TestProbe_StagesShortCircuitOnFirstFailure(t *testing.T) {
	name := "tp-short-circuit"
	defer resetProviderMetrics(name)

	fake := &FakeProvider{
		NameVal: name,
		FindIDFn: func(_ context.Context, _ domain.AnimeRef) (string, error) {
			return "", errors.New("upstream down")
		},
	}
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, _ := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)

	r.RunOnce(context.Background())

	if fake.ListEpisodesCalls() != 0 {
		t.Errorf("ListEpisodes was called %d times after FindID failure; want 0", fake.ListEpisodesCalls())
	}
	if fake.ListServersCalls() != 0 {
		t.Errorf("ListServers was called %d times after FindID failure; want 0", fake.ListServersCalls())
	}
	if fake.GetStreamCalls() != 0 {
		t.Errorf("GetStream was called %d times after FindID failure; want 0", fake.GetStreamCalls())
	}
	// search gauge stays 1 (only 1 failure, threshold is 3).
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, StageSearch)); got != 1 {
		t.Errorf("provider_health_up{stage=search} after 1 failure = %v; want 1 (under threshold)", got)
	}
	// Downstream stages are explicitly flipped to 0 by markRemainingStale.
	// Without this, the gauge would carry stale-OK indefinitely after the
	// upstream stage fails — exactly the silent-decay bug operators noticed
	// in 2026-05 when gogoanime search broke but episodes/servers/stream
	// stayed green for days.
	for _, s := range []string{StageEpisodes, StageServers, StageStream, StageStreamSegment} {
		if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, s)); got != 0 {
			t.Errorf("provider_health_up{stage=%s} = %v; want 0 (downstream of failed search)", s, got)
		}
	}
	// Cache should carry the stale-marker on downstream stages, with a
	// LastErr that names the actually-failed upstream stage so the admin
	// dashboard can render the cause clearly.
	snap := cache.AdminSnapshot()
	ph, ok := snap[name]
	if !ok {
		t.Fatalf("cache has no entry for %q", name)
	}
	wantMsg := `skipped: upstream stage "search" failed this tick`
	for _, s := range []string{StageEpisodes, StageServers, StageStream, StageStreamSegment} {
		st := ph.Stages[s]
		if st.Up {
			t.Errorf("stage %q has Up=true after short-circuit; want false", s)
		}
		if st.LastErr != wantMsg {
			t.Errorf("stage %q LastErr = %q; want %q", s, st.LastErr, wantMsg)
		}
	}
}

// TestProbe_AllFiveStagesEmitGauge_OnFullSuccess — every stage succeeds →
// every gauge is 1, AdminSnapshot has 5 stage entries with LastOK set.
func TestProbe_AllFiveStagesEmitGauge_OnFullSuccess(t *testing.T) {
	name := "tp-all-success"
	defer resetProviderMetrics(name)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 8192))
	}))
	defer srv.Close()

	fake := fullSuccessProvider(name, srv.URL)
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, _ := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)
	allowPrivateHostsForTest(r) // BLK-01: httptest binds 127.0.0.1 (WR-NEW-02: test-only helper)

	r.RunOnce(context.Background())

	for _, s := range AllStages {
		if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues(name, s)); got != 1 {
			t.Errorf("provider_health_up{stage=%s} = %v; want 1", s, got)
		}
	}
	snap := cache.AdminSnapshot()
	ph, ok := snap[name]
	if !ok {
		t.Fatalf("cache has no entry for %q", name)
	}
	if len(ph.Stages) != 5 {
		t.Errorf("cache entry has %d stages; want 5", len(ph.Stages))
	}
	for _, s := range AllStages {
		st := ph.Stages[s]
		if st.LastOK.IsZero() {
			t.Errorf("stage %q has zero LastOK", s)
		}
		if !st.Up {
			t.Errorf("stage %q has Up=false", s)
		}
	}
}

// TestProbe_FetchSegmentRejectsPrivateHost — BLK-01 regression. The probe
// MUST refuse to issue HTTP I/O against loopback / RFC-1918 / docker-internal
// hostnames, even when the upstream pipeline somehow returns such a URL.
// Without this guard a compromised extractor could turn the probe into an
// internal-network SSRF oracle (e.g. http://auth:8080/internal/...).
func TestProbe_FetchSegmentRejectsPrivateHost(t *testing.T) {
	name := "tp-ssrf"
	defer resetProviderMetrics(name)

	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, _ := freshClock(t0)
	// NOTE: no WithAllowPrivateHosts here — we want the production guard.
	r := NewProbeRunner(&FakeProvider{NameVal: name}, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)

	cases := []string{
		"http://auth:8080/internal/resolve-api-key", // docker-internal service
		"http://127.0.0.1:5432/",                    // loopback
		"http://10.0.0.5/",                          // RFC-1918
		"http://192.168.1.1/",                       // RFC-1918
		"http://169.254.169.254/latest/meta-data/",  // link-local (cloud metadata)
		"http://[::1]/",                             // IPv6 loopback
		"file:///etc/passwd",                        // non-http scheme
		"gopher://internal/x",                       // exotic scheme
	}
	for _, urlStr := range cases {
		err := r.fetchSegment(context.Background(), urlStr)
		if err == nil {
			t.Errorf("fetchSegment(%q) returned nil; want SSRF rejection error", urlStr)
		}
	}
}

// TestProbe_HTTPClientRefusesRedirects — BLK-01 regression. The probe client's
// CheckRedirect MUST refuse to follow 3xx Location bounces. Otherwise an
// upstream returning 302 → http://postgres:5432 would let an attacker probe
// internal services via the redirect.
func TestProbe_HTTPClientRefusesRedirects(t *testing.T) {
	name := "tp-redirect"
	defer resetProviderMetrics(name)

	// Backend that bounces every request to an internal-looking destination.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://auth:8080/internal/resolve-api-key")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, _ := freshClock(t0)
	r := NewProbeRunner(&FakeProvider{NameVal: name}, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)
	allowPrivateHostsForTest(r) // allow the httptest.Server itself (WR-NEW-02: test-only helper)

	err := r.fetchSegment(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("fetchSegment returned nil; want redirect-not-followed error")
	}
	// We accept any "status 3xx" failure — the precise wording is in the
	// error message but the contract is "non-2xx → fail".
	if !strings.Contains(err.Error(), "status 3") {
		t.Errorf("fetchSegment err = %v; want a 3xx-status failure", err)
	}
}

// TestProbe_FatalPanicDoesNotRespawn — BLK-03 regression, hardened per
// REVIEW.md iter-2 WR-NEW-01. The previous version of this test ran a
// synthetic anonymous goroutine that panicked + recovered, then counted
// goroutines — which only verified a property of the Go runtime, not the
// production code. A future regression that reintroduces
// `go r.Start(ctx)` into `Start`'s outer recover would NOT have been
// caught.
//
// This version drives the production `Start()` path directly. It uses
// the `withComputeInitialDelayForTest` test-only injection seam to make
// `Start` panic from inside its real loop body, then asserts that
// `Start` RETURNS (i.e. the outer defer-recover did NOT respawn the
// goroutine via `go r.Start(ctx)`).
func TestProbe_FatalPanicDoesNotRespawn(t *testing.T) {
	name := "tp-fatal-panic"
	defer resetProviderMetrics(name)

	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)

	// Make computeInitialDelay panic the first time Start invokes it.
	// The production outer defer-recover sees this panic. If the recover
	// silently calls `go r.Start(ctx)` again, that respawned Start would
	// also panic immediately — and Start would never return. Our select
	// on `done` would then time out, failing the test.
	var calls atomic.Int32
	panicHook := func() time.Duration {
		calls.Add(1)
		panic("WR-NEW-01: injected panic inside Start loop body")
	}

	r := NewProbeRunner(
		fullSuccessProvider(name, "https://example.invalid/seg"),
		DefaultGoldenPool, cache, log,
		WithNow(time.Now),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
		withComputeInitialDelayForTest(panicHook),
	)
	allowPrivateHostsForTest(r)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		// Note: NOT `defer close(done)` inside a wrapping recover. We want
		// the actual r.Start path. r.Start has its OWN outer defer-recover
		// (the contract under test). If that recover ever respawns the
		// goroutine, Start will not return and `done` will never close.
		r.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — Start returned cleanly after the outer recover absorbed
		// the injected panic. The outer recover did NOT respawn the
		// goroutine.
	case <-time.After(1 * time.Second):
		t.Fatalf("Start did not return after injected panic — outer recover may have respawned (calls observed: %d)", calls.Load())
	}

	// Exactly one call into the panic hook: Start invoked
	// computeInitialDelayFn once, panicked, the outer recover absorbed
	// it, and Start returned. A respawn-on-panic regression would
	// produce >1 calls (each respawned Start also calls the hook).
	if n := calls.Load(); n != 1 {
		t.Errorf("panic hook called %d times; want exactly 1 (>1 implies outer recover respawned Start)", n)
	}
}

// TestProbe_NextSleepClamp — WR-07 regression. nextSleep MUST never return
// a duration shorter than probeBaseInterval/2 regardless of how the RNG
// behaves (defensive lower bound).
func TestProbe_NextSleepClamp(t *testing.T) {
	for i := 0; i < 1000; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), 0))
		got := nextSleep(rng)
		if got < probeBaseInterval/2 {
			t.Fatalf("nextSleep iteration %d returned %v; want >= %v",
				i, got, probeBaseInterval/2)
		}
	}
}

// TestProbe_LastErrTruncatedTo256Chars — provider returns a 300-char error;
// cached LastErr is <= 256.
func TestProbe_LastErrTruncatedTo256Chars(t *testing.T) {
	name := "tp-truncate"
	defer resetProviderMetrics(name)

	longMsg := strings.Repeat("x", 300)
	fake := &FakeProvider{
		NameVal: name,
		FindIDFn: func(_ context.Context, _ domain.AnimeRef) (string, error) {
			return "", errors.New(longMsg)
		},
	}
	cache := NewInMemoryHealthCache()
	log := newProbeTestLogger(t)
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	nowFn, _ := freshClock(t0)
	r := NewProbeRunner(fake, DefaultGoldenPool, cache, log,
		WithNow(nowFn),
		WithRNG(rand.New(rand.NewPCG(42, 0))),
	)

	r.RunOnce(context.Background())

	snap := cache.AdminSnapshot()
	ph, ok := snap[name]
	if !ok {
		t.Fatalf("cache has no entry for %q", name)
	}
	st := ph.Stages[StageSearch]
	if len(st.LastErr) > MaxLastErrChars {
		t.Errorf("LastErr len = %d; want <= %d", len(st.LastErr), MaxLastErrChars)
	}
	if len(st.LastErr) == 0 {
		t.Errorf("LastErr unexpectedly empty for failure path")
	}
}
