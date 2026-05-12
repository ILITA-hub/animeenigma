package animekai

// client_test.go — stub-mode tests for animekai.Provider (Phase 19 — ESCAPE
// HATCH). These tests lock in the escape-hatch contract: every Provider
// method returns wrapped domain.ErrProviderDown so the orchestrator treats
// it as a soft skip. When the v3.1 fill-in PR lands, these tests are
// replaced wholesale by goldens-based tests mirroring gogoanime/client_test.go.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// newStubProvider constructs a Provider with real-but-minimal Deps. The
// escape-hatch stub never makes HTTP calls, so the BaseHTTPClient is just a
// placeholder; the Registry is empty (no extractors needed); the Cache is
// the in-memory fakeCache from helpers_test.go. MalSync is the exported
// no-op sentinel — the stub provider does not call Lookup on the success
// path, but New() rejects nil to close the v3.1 fill-in nil-pointer
// footgun (WR-01).
func newStubProvider(t *testing.T) *Provider {
	t.Helper()
	log := logger.Default()
	httpClient := domain.NewBaseHTTPClient(log)
	registry := domain.NewRegistry()
	p, err := New(Deps{
		HTTP:    httpClient,
		Embeds:  registry,
		Cache:   newFakeCache(),
		MalSync: NewNoopMalSync(),
		Log:     log,
	})
	if err != nil {
		t.Fatalf("newStubProvider: New() error = %v; want nil", err)
	}
	return p
}

// Test 1 — Name() returns the stable slug "animekai".
func TestProvider_Name_ReturnsAnimeKai(t *testing.T) {
	p := newStubProvider(t)
	if got, want := p.Name(), "animekai"; got != want {
		t.Fatalf("Name() = %q; want %q", got, want)
	}
}

// Test 2 — FindID returns wrapped ErrProviderDown with the expected message.
func TestProvider_FindID_StubReturnsErrProviderDown(t *testing.T) {
	p := newStubProvider(t)
	id, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Naruto", ShikimoriID: "20"})
	if id != "" {
		t.Errorf("FindID returned id = %q; want empty string", id)
	}
	if err == nil {
		t.Fatal("FindID error = nil; want non-nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("errors.Is(err, domain.ErrProviderDown) = false; want true (err=%v)", err)
	}
	if !errors.Is(err, errAnimeKaiStub) {
		t.Errorf("errors.Is(err, errAnimeKaiStub) = false; want true (err=%v)", err)
	}
	if !strings.Contains(err.Error(), "animekai: FindID not implemented") {
		t.Errorf("err.Error() = %q; want substring 'animekai: FindID not implemented'", err.Error())
	}
}

// Test 3 — ListEpisodes returns wrapped ErrProviderDown.
func TestProvider_ListEpisodes_StubReturnsErrProviderDown(t *testing.T) {
	p := newStubProvider(t)
	eps, err := p.ListEpisodes(context.Background(), "any-slug")
	if eps != nil {
		t.Errorf("ListEpisodes returned eps = %v; want nil", eps)
	}
	if err == nil {
		t.Fatal("ListEpisodes error = nil; want non-nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("errors.Is(err, domain.ErrProviderDown) = false; want true (err=%v)", err)
	}
}

// Test 4 — ListServers returns wrapped ErrProviderDown.
func TestProvider_ListServers_StubReturnsErrProviderDown(t *testing.T) {
	p := newStubProvider(t)
	servers, err := p.ListServers(context.Background(), "slug", "ep-1")
	if servers != nil {
		t.Errorf("ListServers returned servers = %v; want nil", servers)
	}
	if err == nil {
		t.Fatal("ListServers error = nil; want non-nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("errors.Is(err, domain.ErrProviderDown) = false; want true (err=%v)", err)
	}
}

// Test 5 — GetStream returns wrapped ErrProviderDown.
func TestProvider_GetStream_StubReturnsErrProviderDown(t *testing.T) {
	p := newStubProvider(t)
	stream, err := p.GetStream(context.Background(), "slug", "ep-1", "srv-1", domain.CategorySub)
	if stream != nil {
		t.Errorf("GetStream returned stream = %v; want nil", stream)
	}
	if err == nil {
		t.Fatal("GetStream error = nil; want non-nil")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("errors.Is(err, domain.ErrProviderDown) = false; want true (err=%v)", err)
	}
}

// Test 6 — HealthCheck reports all FIVE canonical stages (WR-04) as DOWN
// at boot (escape-hatch invariant: Grafana must NOT see a green panel for
// the ~15 min before the first probe tick fires). The stub MUST include
// stream_segment so the in-memory snapshot mirrors the metric surface
// seeded by main.go — otherwise the two surfaces disagree on which stages
// exist for animekai.
func TestProvider_HealthCheck_AllStagesDownAtBoot(t *testing.T) {
	p := newStubProvider(t)
	h := p.HealthCheck(context.Background())
	if h.Provider != "animekai" {
		t.Errorf("Health.Provider = %q; want \"animekai\"", h.Provider)
	}
	wantStages := health.AllStages
	if got, want := len(h.Stages), len(wantStages); got != want {
		t.Fatalf("len(Stages) = %d; want %d (stages=%v)", got, want, h.Stages)
	}
	for _, stage := range wantStages {
		sh, ok := h.Stages[stage]
		if !ok {
			t.Errorf("Stages missing key %q", stage)
			continue
		}
		if sh.Up {
			t.Errorf("Stages[%q].Up = true; want false at boot (escape-hatch invariant)", stage)
		}
		if !strings.Contains(sh.LastErr, "escape-hatch") {
			t.Errorf("Stages[%q].LastErr = %q; want substring 'escape-hatch'", stage, sh.LastErr)
		}
	}
}

// TestProvider_StageNames_MatchesHealthAllStages — WR-04 regression guard.
// stageNames MUST equal health.AllStages so the in-memory snapshot returned
// by HealthCheck matches the metric surface seeded by main.go. If a future
// stage is added to health.AllStages (e.g. "stream_decrypt"), this test
// breaks here so the maintainer knows the escape-hatch provider also needs
// to mark it down at boot.
func TestProvider_StageNames_MatchesHealthAllStages(t *testing.T) {
	if got, want := len(stageNames), len(health.AllStages); got != want {
		t.Fatalf("len(stageNames) = %d; want %d (must mirror health.AllStages so escape-hatch invariant holds end-to-end)", got, want)
	}
	for i, s := range health.AllStages {
		if stageNames[i] != s {
			t.Errorf("stageNames[%d] = %q; want %q (must equal health.AllStages[%d])", i, stageNames[i], s, i)
		}
	}
}

// Test 7 — New() validates required deps. HTTP/Embeds/Cache/MalSync are all
// required (WR-01). MalSync is required even though the stub never calls
// Lookup, so a v3.1 fill-in PR that adds body-only logic to FindID cannot
// silently land a nil-pointer-deref footgun.
func TestProvider_New_RequiresAllDeps(t *testing.T) {
	log := logger.Default()
	httpClient := domain.NewBaseHTTPClient(log)
	registry := domain.NewRegistry()
	fc := newFakeCache()
	ms := NewNoopMalSync()

	// Empty Deps -> error (missing HTTP).
	if _, err := New(Deps{}); err == nil {
		t.Error("New(Deps{}) error = nil; want non-nil (missing HTTP)")
	}
	// Missing Embeds -> error.
	if _, err := New(Deps{HTTP: httpClient}); err == nil {
		t.Error("New(Deps{HTTP only}) error = nil; want non-nil (missing Embeds)")
	}
	// Missing Cache -> error.
	if _, err := New(Deps{HTTP: httpClient, Embeds: registry}); err == nil {
		t.Error("New(Deps{HTTP+Embeds only}) error = nil; want non-nil (missing Cache)")
	}
	// WR-01: missing MalSync -> error. The error message names the field
	// and points at the NewNoopMalSync() helper so a maintainer who hits
	// this at boot can fix the wiring in one line.
	_, err := New(Deps{HTTP: httpClient, Embeds: registry, Cache: fc})
	if err == nil {
		t.Error("New(Deps{HTTP+Embeds+Cache only, no MalSync}) error = nil; want non-nil (missing MalSync — WR-01 footgun guard)")
	} else if !strings.Contains(err.Error(), "MalSync") {
		t.Errorf("New() error %q must mention MalSync field (WR-01 actionable diagnostic)", err.Error())
	}
	// All required deps set (including non-nil MalSync) -> no error.
	p, err := New(Deps{HTTP: httpClient, Embeds: registry, Cache: fc, MalSync: ms})
	if err != nil {
		t.Fatalf("New(valid Deps with NewNoopMalSync) error = %v; want nil", err)
	}
	if p == nil {
		t.Fatal("New(valid Deps) returned nil provider")
	}
}

// Test 8 — compile-time interface assertion lives in client.go
// (`var _ domain.Provider = (*Provider)(nil)`). If Provider drifts from
// the domain.Provider interface, the package fails to build. This test
// body is a no-op marker so the test count matches the plan.
func TestProvider_ConformsToInterface(t *testing.T) {
	// compile-time check in client.go: var _ domain.Provider = (*Provider)(nil)
	// If this test compiles, the assertion holds.
	var _ domain.Provider = (*Provider)(nil)
}
