package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCollapseByFranchise(t *testing.T) {
	in := []*domain.Anime{
		{ID: "jjk1", Franchise: "jujutsu_kaisen"}, // earliest first (repo orders aired_on ASC)
		{ID: "jjk2", Franchise: "jujutsu_kaisen"},
		{ID: "standalone-a", Franchise: ""},
		{ID: "standalone-b", Franchise: ""},
		{ID: "frieren", Franchise: "frieren"},
	}
	out := collapseByFranchise(in)

	var ids []string
	for _, a := range out {
		ids = append(ids, a.ID)
	}
	want := []string{"jjk1", "standalone-a", "standalone-b", "frieren"}
	if len(ids) != len(want) {
		t.Fatalf("want %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("at %d want %s, got %s (full %v)", i, want[i], ids[i], ids)
		}
	}
}

type fakePoolRepo struct {
	candidates []*domain.Anime
	setCalls   map[string]string // animeID -> franchise
}

func (f *fakePoolRepo) ListGuessPoolCandidates(_ context.Context, _ float64) ([]*domain.Anime, error) {
	return f.candidates, nil
}
func (f *fakePoolRepo) SetFranchise(_ context.Context, id, franchise string) error {
	if f.setCalls == nil {
		f.setCalls = map[string]string{}
	}
	f.setCalls[id] = franchise
	return nil
}

type fakeFranchiseFetcher struct {
	byShikimori map[string]string
	calls       int
}

func (f *fakeFranchiseFetcher) GetAnimeFranchise(_ context.Context, sid string) (string, error) {
	f.calls++
	return f.byShikimori[sid], nil
}

func TestBuildPool_BackfillsAndCollapses(t *testing.T) {
	repo := &fakePoolRepo{candidates: []*domain.Anime{
		// jjk1 already has franchise; jjk2 missing -> backfilled to same franchise -> collapsed away
		{ID: "jjk1", ShikimoriID: "40748", Franchise: "jujutsu_kaisen", NameRU: "Маг. битва",
			Year: 2020, EpisodesCount: 24, Score: 8.6, Status: domain.StatusReleased, Rating: "pg_13",
			Genres:  []domain.Genre{{ID: "1", NameRU: "Экшен"}},
			Studios: []domain.Studio{{ID: "s1", Name: "MAPPA"}},
			Tags:    []domain.Tag{{ID: "t1", Name: "Магия"}}},
		{ID: "jjk2", ShikimoriID: "51009", Franchise: "", NameRU: "Маг. битва 2"},
		{ID: "frieren", ShikimoriID: "52991", Franchise: "", NameRU: "Фрирен", Year: 2023,
			EpisodesCount: 28, Score: 9.3, Status: domain.StatusReleased},
	}}
	fetcher := &fakeFranchiseFetcher{byShikimori: map[string]string{
		"51009": "jujutsu_kaisen", // jjk2 belongs to same franchise
		"52991": "",               // frieren standalone
	}}
	svc := NewGuessPoolService(repo, fetcher, nil)

	entries, err := svc.BuildPool(context.Background())
	if err != nil {
		t.Fatalf("BuildPool: %v", err)
	}
	// jjk2 collapsed into jjk1; frieren stays -> 2 entries
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d (%+v)", len(entries), entries)
	}
	if entries[0].ID != "jjk1" || entries[1].ID != "frieren" {
		t.Fatalf("unexpected entry ids: %s, %s", entries[0].ID, entries[1].ID)
	}
	// jjk2 franchise was backfilled & persisted
	if repo.setCalls["jjk2"] != "jujutsu_kaisen" {
		t.Fatalf("expected jjk2 franchise persisted, got %v", repo.setCalls)
	}
	// frieren is standalone (empty franchise) but must STILL be persisted so it
	// is marked checked and not re-fetched on the next build.
	if _, ok := repo.setCalls["frieren"]; !ok {
		t.Fatalf("expected frieren marked checked via SetFranchise, got %v", repo.setCalls)
	}
	// attribute mapping check on jjk1
	e := entries[0]
	if e.Status != "released" || e.Rating != "pg_13" || e.Score != 8.6 {
		t.Fatalf("bad scalar mapping: %+v", e)
	}
	if len(e.Genres) != 1 || e.Genres[0].Name != "Экшен" {
		t.Fatalf("bad genre mapping: %+v", e.Genres)
	}
	if len(e.Studios) != 1 || e.Studios[0].Name != "MAPPA" {
		t.Fatalf("bad studio mapping: %+v", e.Studios)
	}
	if len(e.Tags) != 1 || e.Tags[0].Name != "Магия" {
		t.Fatalf("bad tag mapping: %+v", e.Tags)
	}
}

// TestBuildPool_SkipsAlreadyChecked ensures a standalone anime that was already
// checked (FranchiseChecked=true, empty franchise) is NOT re-fetched, so the
// pool build does not hammer Shikimori on every call.
func TestBuildPool_SkipsAlreadyChecked(t *testing.T) {
	repo := &fakePoolRepo{candidates: []*domain.Anime{
		{ID: "s1", ShikimoriID: "111", Franchise: "", FranchiseChecked: true, NameRU: "Standalone checked"},
	}}
	fetcher := &fakeFranchiseFetcher{byShikimori: map[string]string{}}
	svc := NewGuessPoolService(repo, fetcher, nil)

	entries, err := svc.BuildPool(context.Background())
	if err != nil {
		t.Fatalf("BuildPool: %v", err)
	}
	if fetcher.calls != 0 {
		t.Fatalf("expected 0 fetcher calls for an already-checked anime, got %d", fetcher.calls)
	}
	if len(entries) != 1 || entries[0].ID != "s1" {
		t.Fatalf("expected the checked standalone in the pool, got %+v", entries)
	}
}
