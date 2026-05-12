package health

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// TestCache_NoEntry_FailsOpen — RESEARCH P-08 fail-open semantic.
// A probe outage MUST NOT blank the service: if the cache has no entry for a
// provider, IsHealthy returns true and the orchestrator dispatches normally.
func TestCache_NoEntry_FailsOpen(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	if !c.IsHealthy("never_probed") {
		t.Errorf("IsHealthy(no entry) = false; want true (fail-open)")
	}
}

// TestCache_FreshUpEntry_ReturnsTrue exercises the happy path: a recent probe
// wrote Up=true for stream_segment.
func TestCache_FreshUpEntry_ReturnsTrue(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	c.Update("animepahe", ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: true}},
		LastUpdated: time.Now(),
	})
	if !c.IsHealthy("animepahe") {
		t.Errorf("IsHealthy(fresh up) = false; want true")
	}
}

// TestCache_FreshDownEntry_ReturnsFalse is the only branch that triggers an
// orchestrator skip.
func TestCache_FreshDownEntry_ReturnsFalse(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	c.Update("animepahe", ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: false, LastErr: "timeout"}},
		LastUpdated: time.Now(),
	})
	if c.IsHealthy("animepahe") {
		t.Errorf("IsHealthy(fresh down) = true; want false")
	}
}

// TestCache_StaleEntry_FailsOpen — stale entry (>60s) is treated as "unknown"
// and fails open. Use the WithNow constructor to advance time past the TTL.
func TestCache_StaleEntry_FailsOpen(t *testing.T) {
	t.Parallel()
	base := time.Now()
	c := NewInMemoryHealthCacheWithNow(func() time.Time {
		return base.Add(70 * time.Second)
	})
	c.Update("animepahe", ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: false}},
		LastUpdated: base, // 70s in the past relative to now()
	})
	if !c.IsHealthy("animepahe") {
		t.Errorf("IsHealthy(stale down) = false; want true (stale = fail-open)")
	}
}

// TestCache_MissingStreamSegmentStage_FailsOpen — the cache only treats a
// provider as DOWN when there is positive evidence (the stream_segment stage
// is present AND Up=false). Missing oracle = unknown = fail-open.
func TestCache_MissingStreamSegmentStage_FailsOpen(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	c.Update("animepahe", ProviderHealth{
		Stages: map[string]StageStatus{
			StageSearch: {Up: false}, // wrong stage — should be ignored
		},
		LastUpdated: time.Now(),
	})
	if !c.IsHealthy("animepahe") {
		t.Errorf("IsHealthy(no stream_segment key) = false; want true (no oracle = fail-open)")
	}
}

// TestCache_ShortCircuitedProbe_FailsOpen_DivergesFromAlerts — REVIEW.md
// WR-03 lock-in test. When the probe short-circuits on an earlier stage
// (e.g. search FindID fails), only that stage gets a Stages map entry.
// stream_segment is absent → IsHealthy returns true (fail-open) so the
// orchestrator keeps dispatching the provider.
//
// This DIVERGES from the alert rule `provider-health-stream-segment-down`
// which only fires on a fresh stream_segment Up=false event. The probe's
// search-stage alert (or any earlier-stage alert) is what pages the
// operator for a fully-broken upstream. This test pins the divergence
// so a future PR that changes IsHealthy semantics must also update the
// alert rules in lockstep.
func TestCache_ShortCircuitedProbe_FailsOpen_DivergesFromAlerts(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	// Simulate what `commit()` writes when the probe short-circuits on
	// the FIRST stage: only `search` is present, the rest are missing
	// because the probe never reached them.
	c.Update("animepahe", ProviderHealth{
		Stages: map[string]StageStatus{
			StageSearch: {Up: false, LastErr: "FindID failed"},
		},
		LastUpdated: time.Now(),
	})
	if !c.IsHealthy("animepahe") {
		t.Errorf("IsHealthy with search=down + missing stream_segment = false; want true (orchestrator gate diverges from alert rule by design — see WR-03)")
	}
}

// TestCache_AdminSnapshot_ReturnsCopy verifies the admin endpoint can mutate
// the returned map without affecting cache state (deep copy semantics).
func TestCache_AdminSnapshot_ReturnsCopy(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	c.Update("animepahe", ProviderHealth{
		Stages: map[string]StageStatus{
			StageStreamSegment: {Up: true},
			StageSearch:        {Up: true},
		},
		LastUpdated: time.Now(),
	})

	snap := c.AdminSnapshot()
	if len(snap) != 1 {
		t.Fatalf("AdminSnapshot len = %d; want 1", len(snap))
	}

	// Mutate the snapshot.
	delete(snap["animepahe"].Stages, StageStreamSegment)
	snap["mutated"] = ProviderHealth{}

	// Original cache must be unaffected.
	if !c.IsHealthy("animepahe") {
		t.Errorf("cache lost stream_segment entry after snapshot mutation — not deep-copied")
	}
	snap2 := c.AdminSnapshot()
	if _, ok := snap2["mutated"]; ok {
		t.Errorf("cache picked up the spurious 'mutated' key — top-level map not copied")
	}
	if _, ok := snap2["animepahe"].Stages[StageStreamSegment]; !ok {
		t.Errorf("cache lost stream_segment entry — Stages map not deep-copied")
	}
}

// TestCache_ConcurrentReadersAndWriter_NoRace — 100 readers + 1 writer running
// concurrently must not race (run under `go test -race`).
func TestCache_ConcurrentReadersAndWriter_NoRace(t *testing.T) {
	t.Parallel()
	c := NewInMemoryHealthCache()
	c.Update("animepahe", ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: true}},
		LastUpdated: time.Now(),
	})

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Spawn 100 readers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = c.IsHealthy("animepahe")
					_ = c.AdminSnapshot()
				}
			}
		}()
	}

	// One writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		flip := true
		for {
			select {
			case <-stop:
				return
			default:
				c.Update("animepahe", ProviderHealth{
					Stages:      map[string]StageStatus{StageStreamSegment: {Up: flip}},
					LastUpdated: time.Now(),
				})
				flip = !flip
			}
		}
	}()

	// Let them race for 50ms — long enough for -race to catch real bugs.
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestFakeProvider_SatisfiesProviderInterface — compile-time guard via assertion
// inside the test (the assertion also exists at package scope in
// testutil_provider.go; we mirror it here so a regression to the test fake's
// shape fails this test, not the whole package build).
func TestFakeProvider_SatisfiesProviderInterface(t *testing.T) {
	t.Parallel()
	var p domain.Provider = &FakeProvider{NameVal: "compile_check"}
	if p.Name() != "compile_check" {
		t.Errorf("FakeProvider.Name() = %q; want compile_check", p.Name())
	}
	// Exercise the default zero-value method paths.
	if id, err := p.FindID(context.Background(), domain.AnimeRef{}); id != "" || err != nil {
		t.Errorf("FindID default = (%q, %v); want (\"\", nil)", id, err)
	}
	if eps, err := p.ListEpisodes(context.Background(), "x"); eps != nil || err != nil {
		t.Errorf("ListEpisodes default = (%v, %v); want (nil, nil)", eps, err)
	}
	if srv, err := p.ListServers(context.Background(), "x", "y"); srv != nil || err != nil {
		t.Errorf("ListServers default = (%v, %v); want (nil, nil)", srv, err)
	}
	if s, err := p.GetStream(context.Background(), "x", "y", "z", domain.CategorySub); s != nil || err != nil {
		t.Errorf("GetStream default = (%v, %v); want (nil, nil)", s, err)
	}
	if h := p.HealthCheck(context.Background()); h.Provider != "compile_check" {
		t.Errorf("HealthCheck.Provider = %q; want compile_check", h.Provider)
	}
}
