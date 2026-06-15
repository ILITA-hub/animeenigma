package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// fakeJackett is a hand-rolled JackettSearcher.
type fakeJackett struct {
	releases []domain.Release
	err      error
	called   bool
	gotQuery string
}

func (f *fakeJackett) Search(_ context.Context, query string, _ int) ([]domain.Release, error) {
	f.called = true
	f.gotQuery = query
	return f.releases, f.err
}

// fakeFallback is a hand-rolled FallbackSearcher.
type fakeFallback struct {
	result Result
	err    error
	called bool
}

func (f *fakeFallback) FetchAll(_ context.Context, _ SearchParams) (Result, error) {
	f.called = true
	return f.result, f.err
}

func testLog() *logger.Logger { return logger.Default() }

func rel(source string, seeders int) domain.Release {
	return domain.Release{InfoHash: source + "-hash", Source: source, Seeders: seeders}
}

func TestTier_JackettHitShortCircuitsFallback(t *testing.T) {
	j := &fakeJackett{releases: []domain.Release{rel("jackett", 50)}}
	fb := &fakeFallback{}
	tier := NewTieredSearcher(j, fb, testLog())

	res, err := tier.FetchAll(context.Background(), SearchParams{Query: "frieren"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !j.called {
		t.Error("jackett should have been called")
	}
	if fb.called {
		t.Error("fallback should NOT have been called on a jackett hit")
	}
	if len(res.Releases) != 1 || res.Releases[0].Source != "jackett" {
		t.Fatalf("expected jackett release, got %+v", res.Releases)
	}
	if len(res.ProvidersDown) != 0 {
		t.Errorf("ProvidersDown should be empty, got %v", res.ProvidersDown)
	}
}

func TestTier_JackettEmptyFallsThrough(t *testing.T) {
	j := &fakeJackett{releases: nil} // empty, no error
	fb := &fakeFallback{result: Result{Releases: []domain.Release{rel("nyaa", 0)}, ProvidersDown: []string{}}}
	tier := NewTieredSearcher(j, fb, testLog())

	res, err := tier.FetchAll(context.Background(), SearchParams{Query: "obscure"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should run when jackett returns nothing")
	}
	if len(res.Releases) != 1 || res.Releases[0].Source != "nyaa" {
		t.Fatalf("expected nyaa fallback release, got %+v", res.Releases)
	}
	// Jackett did not error, so it must NOT appear in ProvidersDown.
	for _, p := range res.ProvidersDown {
		if p == "jackett" {
			t.Error("jackett must not be in ProvidersDown when it merely returned empty")
		}
	}
}

func TestTier_JackettErrorFallsThroughAndRecordsDown(t *testing.T) {
	j := &fakeJackett{err: errors.New("jackett 502")}
	fb := &fakeFallback{result: Result{Releases: []domain.Release{rel("animetosho", 0)}, ProvidersDown: []string{"nyaa"}}}
	tier := NewTieredSearcher(j, fb, testLog())

	res, err := tier.FetchAll(context.Background(), SearchParams{Query: "frieren"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should run when jackett errors")
	}
	if len(res.ProvidersDown) != 2 || res.ProvidersDown[0] != "jackett" || res.ProvidersDown[1] != "nyaa" {
		t.Fatalf("expected [jackett nyaa] ProvidersDown, got %v", res.ProvidersDown)
	}
}

func TestTier_MALOnlySkipsJackett(t *testing.T) {
	j := &fakeJackett{releases: []domain.Release{rel("jackett", 99)}}
	fb := &fakeFallback{result: Result{Releases: []domain.Release{rel("animetosho", 0)}}}
	tier := NewTieredSearcher(j, fb, testLog())

	// Query empty, MALID set → Jackett has no MAL feed, must be skipped.
	res, err := tier.FetchAll(context.Background(), SearchParams{MALID: 52991})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if j.called {
		t.Error("jackett must be skipped on a MAL-only (empty query) call")
	}
	if !fb.called || res.Releases[0].Source != "animetosho" {
		t.Fatalf("expected animetosho MAL fallback, got %+v", res.Releases)
	}
}

func TestTier_NilJackettPassesThrough(t *testing.T) {
	fb := &fakeFallback{result: Result{Releases: []domain.Release{rel("nyaa", 0)}}}
	tier := NewTieredSearcher(nil, fb, testLog())

	res, err := tier.FetchAll(context.Background(), SearchParams{Query: "frieren"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("nil jackett (disabled) must pass straight through to fallback")
	}
	if len(res.Releases) != 1 || res.Releases[0].Source != "nyaa" {
		t.Fatalf("expected nyaa release, got %+v", res.Releases)
	}
}
