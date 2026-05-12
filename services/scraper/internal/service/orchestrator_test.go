package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeProvider is a swappable domain.Provider for orchestrator contract tests.
// Every method delegates to a *Func field; nil func returns zero values.
type fakeProvider struct {
	nameVal          string
	findIDFn         func(ctx context.Context, ref domain.AnimeRef) (string, error)
	listEpisodesFn   func(ctx context.Context, providerID string) ([]domain.Episode, error)
	listServersFn    func(ctx context.Context, providerID, episodeID string) ([]domain.Server, error)
	getStreamFn      func(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category) (*domain.Stream, error)
	healthCheckFn    func(ctx context.Context) domain.Health
	listEpisodeCalls int32 // for ordering assertions
}

func (f *fakeProvider) Name() string { return f.nameVal }
func (f *fakeProvider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	if f.findIDFn != nil {
		return f.findIDFn(ctx, ref)
	}
	return "", nil
}
func (f *fakeProvider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	atomic.AddInt32(&f.listEpisodeCalls, 1)
	if f.listEpisodesFn != nil {
		return f.listEpisodesFn(ctx, providerID)
	}
	return nil, nil
}
func (f *fakeProvider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	if f.listServersFn != nil {
		return f.listServersFn(ctx, providerID, episodeID)
	}
	return nil, nil
}
func (f *fakeProvider) GetStream(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category) (*domain.Stream, error) {
	if f.getStreamFn != nil {
		return f.getStreamFn(ctx, providerID, episodeID, serverID, cat)
	}
	return nil, nil
}
func (f *fakeProvider) HealthCheck(ctx context.Context) domain.Health {
	if f.healthCheckFn != nil {
		return f.healthCheckFn(ctx)
	}
	return domain.Health{Provider: f.nameVal}
}

func newTestOrchestrator(t *testing.T, providers ...domain.Provider) *Orchestrator {
	t.Helper()
	log := logger.Default()
	// Phase 17: existing tests pass nil cache to preserve Phase 16 behaviour
	// (no skip-unhealthy). Tests that exercise the cache use
	// newTestOrchestratorWithCache below.
	o := NewOrchestrator(log, domain.NewRegistry(), nil)
	for _, p := range providers {
		o.Register(p)
	}
	return o
}

// newTestOrchestratorWithCache constructs an orchestrator with a real
// *health.InMemoryHealthCache. Used by Phase 17 tests that verify the
// skip-unhealthy + rejoin behaviour without monkey-patching globals.
func newTestOrchestratorWithCache(t *testing.T, cache *health.InMemoryHealthCache, providers ...domain.Provider) *Orchestrator {
	t.Helper()
	log := logger.Default()
	o := NewOrchestrator(log, domain.NewRegistry(), cache)
	for _, p := range providers {
		o.Register(p)
	}
	return o
}

// TestOrchestrator_ZeroProviders_ReturnsErrNotFound verifies the zero-provider
// edge case: every business method returns (nil, ErrNotFound) without panicking.
// Phase 15 registers zero providers; this lock is what makes "deploy the
// scraper alone" a safe operation.
func TestOrchestrator_ZeroProviders_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	o := newTestOrchestrator(t)
	ctx := context.Background()

	if _, err := o.ListEpisodes(ctx, "any-id", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ListEpisodes err = %v; want ErrNotFound", err)
	}
	if _, err := o.ListServers(ctx, "any-id", "ep1", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ListServers err = %v; want ErrNotFound", err)
	}
	if _, err := o.GetStream(ctx, "any-id", "ep1", "srv1", domain.CategorySub, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetStream err = %v; want ErrNotFound", err)
	}
}

// TestOrchestrator_ZeroProviders_HealthSnapshotEmpty verifies the snapshot
// returns a non-nil empty map for the zero-provider case. The /scraper/health
// handler JSON-marshals this directly.
func TestOrchestrator_ZeroProviders_HealthSnapshotEmpty(t *testing.T) {
	t.Parallel()
	o := newTestOrchestrator(t)
	snap := o.HealthSnapshot(context.Background())
	if snap == nil {
		t.Fatal("HealthSnapshot = nil; want non-nil empty map")
	}
	if len(snap) != 0 {
		t.Errorf("HealthSnapshot len = %d; want 0", len(snap))
	}
}

// TestOrchestrator_SingleProvider_Passthrough verifies a healthy single
// provider's result is returned verbatim.
func TestOrchestrator_SingleProvider_Passthrough(t *testing.T) {
	t.Parallel()
	want := []domain.Episode{{ID: "ep1", Number: 1, Title: "Pilot"}}
	p := &fakeProvider{
		nameVal: "alpha",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return want, nil
		},
	}
	o := newTestOrchestrator(t, p)
	got, err := o.ListEpisodes(context.Background(), "x", "")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil", err)
	}
	if len(got) != 1 || got[0].ID != "ep1" {
		t.Errorf("ListEpisodes = %+v; want %+v", got, want)
	}
}

// TestOrchestrator_FailoverOnProviderDown verifies the failover loop: provider
// A returns ErrProviderDown, provider B returns success → orchestrator returns
// B's result AND increments parser_fallback_total{from=A,to=B}.
func TestOrchestrator_FailoverOnProviderDown(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_down",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapProviderDown(errors.New("conn refused"), "A")
		},
	}
	pb := &fakeProvider{
		nameVal: "B_ok",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "ep1"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa, pb)

	before := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_down", "B_ok"))
	got, err := o.ListEpisodes(context.Background(), "x", "")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil after failover", err)
	}
	if len(got) != 1 || got[0].ID != "ep1" {
		t.Errorf("ListEpisodes returned %+v; want B's result", got)
	}
	after := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_down", "B_ok"))
	if delta := after - before; delta != 1.0 {
		t.Errorf("parser_fallback_total{from=A_down,to=B_ok} delta = %v; want 1.0", delta)
	}
}

// TestOrchestrator_FailoverOnNotFound verifies ErrNotFound also triggers
// failover (and counter). Real-empty (no error, empty slice) does NOT trigger
// failover — that is a different case, exercised by Passthrough.
func TestOrchestrator_FailoverOnNotFound(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_nf",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.ErrNotFound
		},
	}
	pb := &fakeProvider{
		nameVal: "B_ok2",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "epX"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa, pb)

	before := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_nf", "B_ok2"))
	got, err := o.ListEpisodes(context.Background(), "x", "")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil after NotFound failover", err)
	}
	if len(got) != 1 || got[0].ID != "epX" {
		t.Errorf("ListEpisodes returned %+v; want B's result", got)
	}
	after := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_nf", "B_ok2"))
	if delta := after - before; delta != 1.0 {
		t.Errorf("parser_fallback_total{from=A_nf,to=B_ok2} delta = %v; want 1.0", delta)
	}
}

// TestOrchestrator_AllProvidersDown_ReturnsLastErr verifies that when every
// provider fails with ErrProviderDown, the orchestrator surfaces ErrProviderDown
// (preserving the sentinel through errors.Is). The exact wrapping is loose;
// we only check the sentinel.
func TestOrchestrator_AllProvidersDown_ReturnsLastErr(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_d",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapProviderDown(errors.New("dns"), "A")
		},
	}
	pb := &fakeProvider{
		nameVal: "B_d",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapProviderDown(errors.New("conn"), "B")
		},
	}
	o := newTestOrchestrator(t, pa, pb)
	_, err := o.ListEpisodes(context.Background(), "x", "")
	if err == nil {
		t.Fatal("want err, got nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want errors.Is match ErrProviderDown", err)
	}
}

// TestOrchestrator_AllProvidersNotFound_ReturnsErrNotFound verifies that when
// every provider returns ErrNotFound, the orchestrator surfaces ErrNotFound
// (not ErrProviderDown).
func TestOrchestrator_AllProvidersNotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_nf",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.ErrNotFound
		},
	}
	pb := &fakeProvider{
		nameVal: "B_nf",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.ErrNotFound
		},
	}
	o := newTestOrchestrator(t, pa, pb)
	_, err := o.ListEpisodes(context.Background(), "x", "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v; want ErrNotFound", err)
	}
}

// TestOrchestrator_ContextCancelStopsLoop verifies that a cancelled context
// short-circuits the failover loop BEFORE calling provider B. The cancelled
// context error is returned (NOT ErrProviderDown).
func TestOrchestrator_ContextCancelStopsLoop(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_hang",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			// Honor the parent context: block until cancelled.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	pb := &fakeProvider{
		nameVal: "B_should_not_be_called",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "shouldnt"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa, pb)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := o.ListEpisodes(ctx, "x", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want err on ctx cancel, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.DeadlineExceeded or context.Canceled", err)
	}
	if errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err matched ErrProviderDown; expected pure ctx error")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("ListEpisodes took %v; should bail near ctx deadline", elapsed)
	}
	if atomic.LoadInt32(&pb.listEpisodeCalls) != 0 {
		t.Errorf("provider B was called %d times; want 0 after ctx death",
			atomic.LoadInt32(&pb.listEpisodeCalls))
	}
}

// TestOrchestrator_HealthSnapshotReflectsLatest verifies that HealthSnapshot
// calls HealthCheck on every registered provider on each invocation and
// returns the latest values (no stale cache).
func TestOrchestrator_HealthSnapshotReflectsLatest(t *testing.T) {
	t.Parallel()
	var counter int32

	pa := &fakeProvider{
		nameVal: "A_h",
		healthCheckFn: func(ctx context.Context) domain.Health {
			atomic.AddInt32(&counter, 1)
			return domain.Health{
				Provider: "A_h",
				Stages:   map[string]domain.StageHealth{"find_id": {Up: true}},
			}
		},
	}
	pb := &fakeProvider{
		nameVal: "B_h",
		healthCheckFn: func(ctx context.Context) domain.Health {
			return domain.Health{
				Provider: "B_h",
				Stages:   map[string]domain.StageHealth{"find_id": {Up: false, LastErr: "boom"}},
			}
		},
	}
	o := newTestOrchestrator(t, pa, pb)

	snap1 := o.HealthSnapshot(context.Background())
	if len(snap1) != 2 {
		t.Fatalf("HealthSnapshot len = %d; want 2", len(snap1))
	}
	if snap1["A_h"].Provider != "A_h" {
		t.Errorf("snap1[A_h].Provider = %q; want A_h", snap1["A_h"].Provider)
	}
	if !snap1["A_h"].Stages["find_id"].Up {
		t.Errorf("snap1[A_h].find_id.Up = false; want true")
	}
	if snap1["B_h"].Stages["find_id"].Up {
		t.Errorf("snap1[B_h].find_id.Up = true; want false")
	}

	// Second call should re-invoke HealthCheck (no stale cache for Phase 15).
	_ = o.HealthSnapshot(context.Background())
	if got := atomic.LoadInt32(&counter); got < 2 {
		t.Errorf("HealthCheck call count = %d; want >= 2 (snapshot re-queries)", got)
	}
}

// TestOrchestrator_PreferPriority verifies the `prefer` argument moves the
// matching provider to the front of the failover order. If A is registered
// first and B is preferred, B is tried first.
func TestOrchestrator_PreferPriority(t *testing.T) {
	t.Parallel()
	var firstCalled string
	mark := func(name string) func(ctx context.Context, id string) ([]domain.Episode, error) {
		return func(ctx context.Context, id string) ([]domain.Episode, error) {
			if firstCalled == "" {
				firstCalled = name
			}
			return []domain.Episode{{ID: name}}, nil
		}
	}
	pa := &fakeProvider{nameVal: "A_pref", listEpisodesFn: mark("A_pref")}
	pb := &fakeProvider{nameVal: "B_pref", listEpisodesFn: mark("B_pref")}
	o := newTestOrchestrator(t, pa, pb)

	got, err := o.ListEpisodes(context.Background(), "x", "B_pref")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil", err)
	}
	if firstCalled != "B_pref" {
		t.Errorf("firstCalled = %q; want B_pref (preferred should go first)", firstCalled)
	}
	if len(got) != 1 || got[0].ID != "B_pref" {
		t.Errorf("ListEpisodes returned %+v; want B_pref's result", got)
	}
}

// TestOrchestrator_PreferPriority_NoDuplicates verifies that the preferred
// provider is invoked at most once across a failover-exhaustion scenario.
// REVIEW.md CR-01: a previous version of orderedProviders would append the
// preferred provider twice (once at the front, once again on its natural
// position in the second loop), causing the prefer'd upstream to be hit
// twice on failover. This test locks the contract: each provider is called
// exactly once per business call, regardless of `prefer`.
func TestOrchestrator_PreferPriority_NoDuplicates(t *testing.T) {
	t.Parallel()

	var callsA, callsB, callsC int32

	pa := &fakeProvider{
		nameVal: "A_nd",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			atomic.AddInt32(&callsA, 1)
			return nil, domain.WrapProviderDown(errors.New("A down"), "A")
		},
	}
	pb := &fakeProvider{
		nameVal: "B_nd",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			atomic.AddInt32(&callsB, 1)
			return nil, domain.WrapProviderDown(errors.New("B down"), "B")
		},
	}
	pc := &fakeProvider{
		nameVal: "C_nd",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			atomic.AddInt32(&callsC, 1)
			return nil, domain.WrapProviderDown(errors.New("C down"), "C")
		},
	}
	// Register order: A, B, C. Prefer B → expected order [B, A, C].
	o := newTestOrchestrator(t, pa, pb, pc)

	_, err := o.ListEpisodes(context.Background(), "x", "B_nd")
	if err == nil {
		t.Fatal("ListEpisodes: want err on all-down, got nil")
	}
	if got := atomic.LoadInt32(&callsA); got != 1 {
		t.Errorf("A_nd called %d times; want exactly 1", got)
	}
	if got := atomic.LoadInt32(&callsB); got != 1 {
		t.Errorf("B_nd called %d times; want exactly 1 (preferred MUST NOT duplicate)", got)
	}
	if got := atomic.LoadInt32(&callsC); got != 1 {
		t.Errorf("C_nd called %d times; want exactly 1", got)
	}

	// Also verify the slice returned by orderedProviders has the correct length.
	ordered := o.orderedProviders("B_nd")
	if len(ordered) != 3 {
		t.Errorf("orderedProviders len = %d; want 3 (no duplicates)", len(ordered))
	}
	if ordered[0].Name() != "B_nd" {
		t.Errorf("orderedProviders[0] = %q; want B_nd (preferred first)", ordered[0].Name())
	}
}

// TestOrchestrator_FailoverFallbackTotalIncrementCount verifies the
// *total* number of parser_fallback_total increments across a failover
// loop equals exactly len(providers)-1 (the maximum possible, one per
// failover hop). This catches the failure mode where a buggy iteration
// order causes a provider to be tried twice (CR-01) — the existing
// fallback tests only check single-label deltas and would silently pass
// if the same provider were called twice with the same `from`/`to` pair
// in different orders.
//
// See REVIEW.md WR-08 for context on global-registry metric pollution.
func TestOrchestrator_FailoverFallbackTotalIncrementCount(t *testing.T) {
	t.Parallel()

	pa := &fakeProvider{
		nameVal: "A_count",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapProviderDown(errors.New("A down"), "A")
		},
	}
	pb := &fakeProvider{
		nameVal: "B_count",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapProviderDown(errors.New("B down"), "B")
		},
	}
	pc := &fakeProvider{
		nameVal: "C_count",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "ep"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa, pb, pc)

	// Capture all (from,to) deltas we expect: A→B and B→C.
	beforeAB := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_count", "B_count"))
	beforeBC := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("B_count", "C_count"))

	if _, err := o.ListEpisodes(context.Background(), "x", ""); err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil after C recovers", err)
	}

	afterAB := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_count", "B_count"))
	afterBC := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("B_count", "C_count"))

	if d := afterAB - beforeAB; d != 1.0 {
		t.Errorf("ParserFallbackTotal{from=A_count,to=B_count} delta = %v; want 1.0", d)
	}
	if d := afterBC - beforeBC; d != 1.0 {
		t.Errorf("ParserFallbackTotal{from=B_count,to=C_count} delta = %v; want 1.0", d)
	}

	// Total increments = (afterAB-beforeAB) + (afterBC-beforeBC) must equal
	// len(providers)-1 = 2. Any duplicate failover hop (e.g. from CR-01-style
	// double-iteration) would push this over 2.
	totalDelta := (afterAB - beforeAB) + (afterBC - beforeBC)
	if totalDelta != float64(3-1) {
		t.Errorf("total fallback increments = %v; want %d (len(providers)-1, no duplicates)",
			totalDelta, 3-1)
	}
}

// TestOrchestrator_PreferUnknownIgnored verifies that an unknown `prefer`
// value falls back to default registration order (does not crash, does not
// return ErrNotFound just because the preference was wrong).
func TestOrchestrator_PreferUnknownIgnored(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "A_only",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "A_first"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa)
	got, err := o.ListEpisodes(context.Background(), "x", "unknown_provider")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil", err)
	}
	if len(got) != 1 || got[0].ID != "A_first" {
		t.Errorf("ListEpisodes returned %+v; want A's result", got)
	}
}

// TestOrchestrator_EmbedRegistryAccessor verifies the embed registry is
// reachable from the orchestrator (handlers may need to enumerate Names()
// for /health observability).
func TestOrchestrator_EmbedRegistryAccessor(t *testing.T) {
	t.Parallel()
	o := newTestOrchestrator(t)
	reg := o.EmbedRegistry()
	if reg == nil {
		t.Fatal("EmbedRegistry() = nil; want non-nil registry")
	}
	if names := reg.Names(); len(names) != 0 {
		t.Errorf("Names() on empty registry = %v; want empty", names)
	}
}

// -----------------------------------------------------------------------------
// Phase 17 Plan 01 — orchestrator skip-unhealthy tests (SCRAPER-OBS-03).
// -----------------------------------------------------------------------------

// downCacheEntry is a helper that writes a fresh DOWN entry for `provider`
// into `c` (LastUpdated = now() so it's not stale).
func downCacheEntry(c *health.InMemoryHealthCache, provider string) {
	c.Update(provider, health.ProviderHealth{
		Stages:      map[string]health.StageStatus{health.StageStreamSegment: {Up: false}},
		LastUpdated: time.Now(),
	})
}

// upCacheEntry is the same but Up=true (the "rejoined" state).
func upCacheEntry(c *health.InMemoryHealthCache, provider string) {
	c.Update(provider, health.ProviderHealth{
		Stages:      map[string]health.StageStatus{health.StageStreamSegment: {Up: true}},
		LastUpdated: time.Now(),
	})
}

// TestOrchestrator_NilCache_Backcompat verifies the Phase 16-vs-17 boundary:
// constructing with nil cache MUST preserve the existing dispatch behaviour
// for every failover path (no skipping; providers receive every call).
func TestOrchestrator_NilCache_Backcompat(t *testing.T) {
	t.Parallel()

	p := &fakeProvider{
		nameVal:        "back_compat",
		findIDFn:       func(ctx context.Context, ref domain.AnimeRef) (string, error) { return "id-1", nil },
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) { return []domain.Episode{{ID: "ep1"}}, nil },
		listServersFn:  func(ctx context.Context, pid, eid string) ([]domain.Server, error) { return []domain.Server{{ID: "srv1"}}, nil },
		getStreamFn: func(ctx context.Context, pid, eid, sid string, c domain.Category) (*domain.Stream, error) {
			return &domain.Stream{Sources: []domain.Source{{URL: "u"}}}, nil
		},
	}
	// nil cache — Phase 16 semantics.
	o := newTestOrchestrator(t, p)

	ctx := context.Background()
	if id, err := o.FindID(ctx, domain.AnimeRef{}, ""); err != nil || id != "id-1" {
		t.Errorf("FindID = (%q, %v); want (id-1, nil)", id, err)
	}
	if eps, err := o.ListEpisodes(ctx, "x", ""); err != nil || len(eps) != 1 {
		t.Errorf("ListEpisodes = (%+v, %v); want one episode", eps, err)
	}
	if srvs, err := o.ListServers(ctx, "x", "y", ""); err != nil || len(srvs) != 1 {
		t.Errorf("ListServers = (%+v, %v); want one server", srvs, err)
	}
	if s, err := o.GetStream(ctx, "x", "y", "z", domain.CategorySub, ""); err != nil || s == nil {
		t.Errorf("GetStream = (%+v, %v); want a stream", s, err)
	}
}

// TestOrchestrator_SkipsUnhealthyProvider — the core SCRAPER-OBS-03 contract:
// when the cache reports the first-registered provider DOWN, the orchestrator
// MUST skip it and route the call to the next provider, emitting
// parser_fallback_total{from, to} once.
func TestOrchestrator_SkipsUnhealthyProvider(t *testing.T) {
	t.Parallel()

	cache := health.NewInMemoryHealthCache()
	downCacheEntry(cache, "animepahe_skip")

	pa := &fakeProvider{
		nameVal: "animepahe_skip",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			t.Errorf("animepahe_skip.FindID was called; expected skip")
			return "should-not-happen", nil
		},
	}
	pb := &fakeProvider{
		nameVal: "fallback_provider",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			return "fallback-id", nil
		},
	}
	o := newTestOrchestratorWithCache(t, cache, pa, pb)

	before := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("animepahe_skip", "fallback_provider"))
	id, err := o.FindID(context.Background(), domain.AnimeRef{}, "")
	if err != nil {
		t.Fatalf("FindID err = %v; want nil", err)
	}
	if id != "fallback-id" {
		t.Errorf("FindID = %q; want fallback-id", id)
	}
	after := testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("animepahe_skip", "fallback_provider"))
	if d := after - before; d != 1.0 {
		t.Errorf("parser_fallback_total{from=animepahe_skip, to=fallback_provider} delta = %v; want 1.0", d)
	}
	// Sanity: skipped provider must not have its FindID method invoked.
	// (Asserted via the t.Errorf in the closure above.)
}

// TestOrchestrator_RejoinsHealthyProvider — once the cache flips back to UP,
// the next request reaches the previously-skipped provider.
func TestOrchestrator_RejoinsHealthyProvider(t *testing.T) {
	t.Parallel()

	cache := health.NewInMemoryHealthCache()
	downCacheEntry(cache, "rejoin_provider")

	var callsToA int32
	pa := &fakeProvider{
		nameVal: "rejoin_provider",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			atomic.AddInt32(&callsToA, 1)
			return "rejoined-id", nil
		},
	}
	pb := &fakeProvider{
		nameVal:  "rejoin_fallback",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) { return "fallback-id", nil },
	}
	o := newTestOrchestratorWithCache(t, cache, pa, pb)

	// First call: A is DOWN, B answers.
	id, err := o.FindID(context.Background(), domain.AnimeRef{}, "")
	if err != nil || id != "fallback-id" {
		t.Fatalf("first FindID = (%q, %v); want (fallback-id, nil)", id, err)
	}
	if got := atomic.LoadInt32(&callsToA); got != 0 {
		t.Errorf("after first call, rejoin_provider was called %d times; want 0", got)
	}

	// Flip the cache to UP — next request reaches A.
	upCacheEntry(cache, "rejoin_provider")

	id, err = o.FindID(context.Background(), domain.AnimeRef{}, "")
	if err != nil || id != "rejoined-id" {
		t.Fatalf("second FindID = (%q, %v); want (rejoined-id, nil)", id, err)
	}
	if got := atomic.LoadInt32(&callsToA); got != 1 {
		t.Errorf("after rejoin, rejoin_provider was called %d times; want 1", got)
	}
}

// TestOrchestrator_AllProvidersDown_ReturnsAggregateError — when every
// provider is cache-DOWN, the orchestrator MUST surface ErrProviderDown
// (not ErrNotFound) so callers can correctly distinguish "providers exist
// but are skipped" from "no provider has this anime".
func TestOrchestrator_AllProvidersDown_ReturnsAggregateError(t *testing.T) {
	t.Parallel()

	cache := health.NewInMemoryHealthCache()
	downCacheEntry(cache, "all_down_A")
	downCacheEntry(cache, "all_down_B")

	pa := &fakeProvider{
		nameVal: "all_down_A",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			t.Errorf("all_down_A.FindID was called; expected skip")
			return "", nil
		},
	}
	pb := &fakeProvider{
		nameVal: "all_down_B",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			t.Errorf("all_down_B.FindID was called; expected skip")
			return "", nil
		},
	}
	o := newTestOrchestratorWithCache(t, cache, pa, pb)

	_, err := o.FindID(context.Background(), domain.AnimeRef{}, "")
	if err == nil {
		t.Fatal("FindID err = nil; want non-nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want errors.Is match ErrProviderDown", err)
	}
}

// TestOrchestrator_StaleCache_DoesNotSkip — stale (>60s) DOWN entries are
// treated as unknown and the orchestrator dispatches normally (fail-open).
// This guards against a probe outage silently shutting off all providers.
func TestOrchestrator_StaleCache_DoesNotSkip(t *testing.T) {
	t.Parallel()

	// now() returns "70 seconds in the future" so the entry's LastUpdated
	// (real wall-clock now) is treated as stale.
	staleNow := time.Now().Add(70 * time.Second)
	cache := health.NewInMemoryHealthCacheWithNow(func() time.Time { return staleNow })
	cache.Update("stale_provider", health.ProviderHealth{
		Stages:      map[string]health.StageStatus{health.StageStreamSegment: {Up: false}},
		LastUpdated: time.Now(), // 70s "in the past" from cache's POV
	})

	var called int32
	p := &fakeProvider{
		nameVal: "stale_provider",
		findIDFn: func(ctx context.Context, ref domain.AnimeRef) (string, error) {
			atomic.AddInt32(&called, 1)
			return "stale-id", nil
		},
	}
	o := newTestOrchestratorWithCache(t, cache, p)

	id, err := o.FindID(context.Background(), domain.AnimeRef{}, "")
	if err != nil || id != "stale-id" {
		t.Fatalf("FindID = (%q, %v); want (stale-id, nil)", id, err)
	}
	if got := atomic.LoadInt32(&called); got != 1 {
		t.Errorf("stale_provider was called %d times; want 1 (stale = fail-open, no skip)", got)
	}
}
