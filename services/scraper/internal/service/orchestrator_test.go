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
	o := NewOrchestrator(log, domain.NewRegistry())
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
