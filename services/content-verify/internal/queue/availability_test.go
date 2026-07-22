// Provider-availability gating tests (owner directive 2026-07-20): deferrals
// from upstream 503s and roster health="down" must keep content-verify from
// enumerating or claiming a provider until it is available again.
package queue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// availabilityFixture serves capabilities with TWO scraper providers where
// miruro's episodes endpoint answers 503 (retry_after_seconds=900) and
// gogoanime works, plus a roster route whose health values are settable.
type availabilityFixture struct {
	engine      *Engine
	miruroHits  *int64
	rosterState *atomic.Value // string: miruro health
}

func newAvailabilityFixture(t *testing.T) *availabilityFixture {
	t.Helper()
	miruroHits := new(int64)
	rosterState := new(atomic.Value)
	rosterState.Store("up")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/o1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
			{"provider":"gogoanime","state":"active","group":"en"},
			{"provider":"miruro","state":"active","group":"en"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/scraper/episodes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("prefer") == "miruro" {
			atomic.AddInt64(miruroHits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"success":false,"error":{"code":"PROVIDER_DOWN","message":"down","retry_after_seconds":900},"meta":{"tried":["miruro"]}}`))
			return
		}
		w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/scraper/servers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"}]}}`))
	})
	mux.HandleFunc("/internal/scraper/providers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"gogoanime","health":"up","policy":"auto","scraper_operated":true},
			{"name":"miruro","health":"` + rosterState.Load().(string) + `","policy":"auto","scraper_operated":true}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	e := NewEngine(cat, nil, nil, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, 3, nil)
	return &availabilityFixture{engine: e, miruroHits: miruroHits, rosterState: rosterState}
}

// A 503 during enumeration records a deferral, and the NEXT enumeration
// skips the provider's fetches entirely (no second sidecar-backed call).
func TestEnumerate503DefersProvider(t *testing.T) {
	t.Parallel()
	f := newAvailabilityFixture(t)
	e := f.engine
	ctx := context.Background()

	enum, err := e.enumerate(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range enum.Verify {
		if u.Provider == "miruro" {
			t.Fatalf("miruro must not enumerate units through a 503; got %+v", u)
		}
	}
	if got := atomic.LoadInt64(f.miruroHits); got != 1 {
		t.Fatalf("miruro episodes hits after first enum = %d; want 1", got)
	}
	if !e.deferred("o1", "miruro") {
		t.Fatal("503 must record a deferral for (o1, miruro)")
	}

	// Bust the enum cache and re-enumerate: the deferral must gate the fetch.
	e.mu.Lock()
	delete(e.enumCache, "o1")
	e.mu.Unlock()
	if _, err := e.enumerate(ctx, "o1"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(f.miruroHits); got != 1 {
		t.Errorf("miruro episodes hits after deferred enum = %d; want still 1 (deferral must skip the fetch)", got)
	}
}

// Defer/deferred honor the advertised window against the engine clock.
func TestDeferWindowExpires(t *testing.T) {
	t.Parallel()
	f := newAvailabilityFixture(t)
	e := f.engine
	base := time.Now()
	e.now = func() time.Time { return base }

	e.Defer("o1", "miruro", 900*time.Second)
	if !e.deferred("o1", "miruro") {
		t.Fatal("pair must be deferred inside the window")
	}
	if e.deferred("o1", "gogoanime") {
		t.Fatal("deferral must be scoped to the (anime, provider) pair")
	}
	e.now = func() time.Time { return base.Add(901 * time.Second) }
	if e.deferred("o1", "miruro") {
		t.Fatal("deferral must expire after the advertised window")
	}
}

// Roster health="down" gates units (and recovery lifts the gate after the
// roster TTL).
func TestRosterDownGatesProvider(t *testing.T) {
	t.Parallel()
	f := newAvailabilityFixture(t)
	e := f.engine
	ctx := context.Background()

	f.rosterState.Store("down")
	units := []Unit{
		{AnimeID: "o1", Provider: "miruro", Episode: 1},
		{AnimeID: "o1", Provider: "gogoanime", Episode: 1},
	}
	got := e.filterAvailableUnits(ctx, units)
	if len(got) != 1 || got[0].Provider != "gogoanime" {
		t.Fatalf("filtered units = %+v; want only gogoanime (miruro roster-down)", got)
	}

	// Recovery: flip the roster and jump past rosterTTL — the gate lifts.
	f.rosterState.Store("recovering")
	e.mu.Lock()
	e.rosterAt = e.now().Add(-rosterTTL - time.Second)
	e.mu.Unlock()
	got = e.filterAvailableUnits(ctx, units)
	if len(got) != 2 {
		t.Fatalf("filtered units after recovery = %+v; want both providers", got)
	}
}

// A roster fetch failure fails open — nothing is gated.
func TestRosterFetchFailureFailsOpen(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	e := NewEngine(cat, nil, nil, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, 3, nil)

	if e.providerDown(context.Background(), "miruro") {
		t.Fatal("roster outage must fail open (no provider gated)")
	}
}

// Synth units bypass the gate — they persist provider-native metadata
// without probing anything.
func TestSynthUnitsBypassGate(t *testing.T) {
	t.Parallel()
	f := newAvailabilityFixture(t)
	e := f.engine
	e.Defer("o1", "kodik", time.Hour)

	units := []Unit{{AnimeID: "o1", Provider: "kodik",
		Synth: &domain.UnitVerdict{Status: domain.StatusVerified}}}
	got := e.filterAvailableUnits(context.Background(), units)
	if len(got) != 1 {
		t.Fatalf("synth unit must bypass the availability gate; got %+v", got)
	}
}
