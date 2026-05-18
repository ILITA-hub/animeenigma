package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// fakeNyaa implements NyaaSearcher with canned returns.
type fakeNyaa struct {
	releases []domain.Release
	err      error
}

func (f *fakeNyaa) Search(_ context.Context, _ string, _ int) ([]domain.Release, error) {
	return f.releases, f.err
}

// fakeAT implements AnimeToshoSearcher with canned returns.
type fakeAT struct {
	releases []domain.Release
	err      error
}

func (f *fakeAT) Search(_ context.Context, _ AnimeToshoParams) ([]domain.Release, error) {
	return f.releases, f.err
}

// testLogger returns a no-op logger so tests don't spam stdout.
func testLogger() *logger.Logger {
	return logger.Default()
}

func mkRelease(source, hash string, malID int, when time.Time) domain.Release {
	return domain.Release{
		Title:    source + "-" + hash,
		InfoHash: hash,
		Source:   source,
		MALID:    malID,
		FoundAt:  when,
	}
}

func TestFetchAll_BothSucceed_Dedupes(t *testing.T) {
	t0 := time.Date(2024, 5, 18, 10, 0, 0, 0, time.UTC)
	commonHash := "aabbccddeeff00112233445566778899aabbccdd"

	nyaa := &fakeNyaa{releases: []domain.Release{
		mkRelease("nyaa", commonHash, 0, t0),
	}}
	at := &fakeAT{releases: []domain.Release{
		mkRelease("animetosho", commonHash, 52991, t0.Add(time.Hour)),
	}}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x", MALID: 52991})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(res.Releases) != 1 {
		t.Fatalf("expected 1 deduped release, got %d", len(res.Releases))
	}
	if res.Releases[0].Source != "animetosho" {
		t.Errorf("dedupe winner: want animetosho (richer metadata), got %q", res.Releases[0].Source)
	}
	if len(res.ProvidersDown) != 0 {
		t.Errorf("ProvidersDown: want empty, got %v", res.ProvidersDown)
	}
}

func TestFetchAll_RanksAnimeToshoWithMatchingMAL(t *testing.T) {
	t0 := time.Date(2024, 5, 18, 10, 0, 0, 0, time.UTC)

	nyaa := &fakeNyaa{releases: []domain.Release{
		mkRelease("nyaa", "n1111111111111111111111111111111111111111", 0, t0.Add(2*time.Hour)),
		mkRelease("nyaa", "n2222222222222222222222222222222222222222", 0, t0.Add(3*time.Hour)),
	}}
	at := &fakeAT{releases: []domain.Release{
		// matching MAL ID — should rank above all nyaa entries
		mkRelease("animetosho", "a1111111111111111111111111111111111111111", 52991, t0),
		// non-matching MAL ID (e.g. query-path AT hit) — rank by FoundAt only
		mkRelease("animetosho", "a2222222222222222222222222222222222222222", 0, t0.Add(4*time.Hour)),
	}}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{MALID: 52991})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(res.Releases) != 4 {
		t.Fatalf("expected 4 releases, got %d", len(res.Releases))
	}
	// The single AT-with-matching-MAL hit must be first, regardless of
	// FoundAt (it's the OLDEST entry but ranked first by the MAL gate).
	if res.Releases[0].Source != "animetosho" || res.Releases[0].MALID != 52991 {
		t.Errorf("first hit: want animetosho/MAL=52991, got %+v", res.Releases[0])
	}
}

func TestFetchAll_NyaaDown(t *testing.T) {
	nyaa := &fakeNyaa{err: errors.New("nyaa boom")}
	at := &fakeAT{releases: []domain.Release{
		mkRelease("animetosho", "a1111111111111111111111111111111111111111", 0, time.Now()),
	}}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x"})
	if err != nil {
		t.Fatalf("FetchAll should be soft when one provider is down, got %v", err)
	}
	if len(res.Releases) != 1 {
		t.Errorf("expected 1 surviving release, got %d", len(res.Releases))
	}
	if len(res.ProvidersDown) != 1 || res.ProvidersDown[0] != "nyaa" {
		t.Errorf("ProvidersDown: want [nyaa], got %v", res.ProvidersDown)
	}
}

func TestFetchAll_AnimeToshoDown(t *testing.T) {
	nyaa := &fakeNyaa{releases: []domain.Release{
		mkRelease("nyaa", "n1111111111111111111111111111111111111111", 0, time.Now()),
	}}
	at := &fakeAT{err: errors.New("at boom")}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x"})
	if err != nil {
		t.Fatalf("FetchAll should be soft when one provider is down, got %v", err)
	}
	if len(res.Releases) != 1 {
		t.Errorf("expected 1 surviving release, got %d", len(res.Releases))
	}
	if len(res.ProvidersDown) != 1 || res.ProvidersDown[0] != "animetosho" {
		t.Errorf("ProvidersDown: want [animetosho], got %v", res.ProvidersDown)
	}
}

func TestFetchAll_BothDown(t *testing.T) {
	nyaa := &fakeNyaa{err: errors.New("nyaa down")}
	at := &fakeAT{err: errors.New("at down")}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x"})
	if err == nil {
		t.Fatal("FetchAll should return a non-nil error when BOTH providers fail")
	}
	if len(res.ProvidersDown) != 2 {
		t.Errorf("ProvidersDown: want 2, got %v", res.ProvidersDown)
	}
}

func TestFetchAll_LimitClamp(t *testing.T) {
	now := time.Now()
	makeMany := func(prefix string, n int) []domain.Release {
		out := make([]domain.Release, n)
		for i := 0; i < n; i++ {
			// 40-char unique hashes
			hash := prefix + // 1-char prefix
				// pad to 39 chars after prefix
				"0000000000000000000000000000000000000" +
				string(rune('a'+i%26)) + // 1 char of variability
				string(rune('a'+(i/26)%26)) // another char of variability
			out[i] = mkRelease(prefix, hash, 0, now.Add(time.Duration(i)*time.Second))
		}
		return out
	}

	nyaa := &fakeNyaa{releases: makeMany("n", 30)}
	at := &fakeAT{releases: makeMany("a", 30)}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x", Limit: 5})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(res.Releases) != 5 {
		t.Errorf("expected 5 (limit clamp), got %d", len(res.Releases))
	}
}

func TestFetchAll_EmptyInfoHashSkipped(t *testing.T) {
	now := time.Now()
	nyaa := &fakeNyaa{releases: []domain.Release{
		mkRelease("nyaa", "", 0, now),                                              // dropped
		mkRelease("nyaa", "n1111111111111111111111111111111111111111", 0, now),    // kept
	}}
	at := &fakeAT{releases: []domain.Release{
		mkRelease("animetosho", "", 0, now), // dropped
	}}

	a := NewAggregator(nyaa, at, testLogger())
	res, err := a.FetchAll(context.Background(), SearchParams{Query: "x"})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(res.Releases) != 1 {
		t.Errorf("expected 1 (two empty-hash entries dropped), got %d", len(res.Releases))
	}
}
