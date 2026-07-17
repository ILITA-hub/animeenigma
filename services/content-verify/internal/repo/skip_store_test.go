package repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func skipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.SkipTiming{}, &domain.SkipFingerprint{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpsertSkipReplacesNotAppends(t *testing.T) {
	s := NewStore(skipTestDB(t))
	ctx := context.Background()

	t1 := domain.SkipTiming{
		AnimeID: "a-1", Provider: "gogoanime", Team: "", Episode: 1,
		OpStart: 10, OpEnd: 100, OpStatus: domain.SkipDetected, Confidence: 0.9,
	}
	if err := s.UpsertSkip(ctx, t1); err != nil {
		t.Fatal(err)
	}

	// Re-upsert the same unit key (anime, provider, team, episode) with new
	// statuses/timings → must still be one row, with updated values.
	t2 := domain.SkipTiming{
		AnimeID: "a-1", Provider: "gogoanime", Team: "", Episode: 1,
		OpStart: 12, OpEnd: 102, OpStatus: domain.SkipNoMatch, Confidence: 0.4,
	}
	if err := s.UpsertSkip(ctx, t2); err != nil {
		t.Fatal(err)
	}

	rows, err := s.SkipByAnime(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(rows), rows)
	}
	if rows[0].OpStatus != domain.SkipNoMatch || rows[0].OpStart != 12 || rows[0].Confidence != 0.4 {
		t.Fatalf("upsert did not update in place: %+v", rows[0])
	}

	// A different unit key (different episode) → appended, not merged.
	t3 := domain.SkipTiming{
		AnimeID: "a-1", Provider: "gogoanime", Team: "", Episode: 2,
		OpStatus: domain.SkipPendingFP,
	}
	if err := s.UpsertSkip(ctx, t3); err != nil {
		t.Fatal(err)
	}
	// A different provider, same episode → also appended.
	t4 := domain.SkipTiming{
		AnimeID: "a-1", Provider: "animepahe", Team: "", Episode: 1,
		OpStatus: domain.SkipUnreachable,
	}
	if err := s.UpsertSkip(ctx, t4); err != nil {
		t.Fatal(err)
	}

	rows, err = s.SkipByAnime(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %+v", len(rows), rows)
	}
}

func TestSkipByAnimeOrdering(t *testing.T) {
	s := NewStore(skipTestDB(t))
	ctx := context.Background()

	// Insert out of order; SkipByAnime must return provider, team, episode
	// ascending.
	unordered := []domain.SkipTiming{
		{AnimeID: "a-1", Provider: "gogoanime", Team: "", Episode: 2},
		{AnimeID: "a-1", Provider: "animepahe", Team: "610", Episode: 1},
		{AnimeID: "a-1", Provider: "gogoanime", Team: "", Episode: 1},
		{AnimeID: "a-1", Provider: "animepahe", Team: "100", Episode: 1},
		// Different anime — must not leak into a-1's results.
		{AnimeID: "a-2", Provider: "gogoanime", Team: "", Episode: 1},
	}
	for _, row := range unordered {
		if err := s.UpsertSkip(ctx, row); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := s.SkipByAnime(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4: %+v", len(rows), rows)
	}
	type key struct {
		provider string
		team     string
		episode  int
	}
	want := []key{
		{"animepahe", "100", 1},
		{"animepahe", "610", 1},
		{"gogoanime", "", 1},
		{"gogoanime", "", 2},
	}
	for i, row := range rows {
		got := key{row.Provider, row.Team, row.Episode}
		if got != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestAddFingerprintAndFingerprintsRoundtrip(t *testing.T) {
	s := NewStore(skipTestDB(t))
	ctx := context.Background()

	fp1 := domain.SkipFingerprint{
		AnimeID: "a-1", Kind: domain.SkipKindOp,
		Fp: domain.FpInts{1, 2, 3, 4294967295}, Length: 90.5, SourceNote: "team=610 ep=1",
	}
	if err := s.AddFingerprint(ctx, fp1); err != nil {
		t.Fatal(err)
	}
	fp2 := domain.SkipFingerprint{
		AnimeID: "a-1", Kind: domain.SkipKindEd,
		Fp: domain.FpInts{9, 9, 9}, Length: 30, SourceNote: "team=610 ep=1",
	}
	if err := s.AddFingerprint(ctx, fp2); err != nil {
		t.Fatal(err)
	}
	// Different anime — must not leak into a-1's results.
	other := domain.SkipFingerprint{AnimeID: "a-2", Kind: domain.SkipKindOp, Fp: domain.FpInts{7}}
	if err := s.AddFingerprint(ctx, other); err != nil {
		t.Fatal(err)
	}

	got, err := s.Fingerprints(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("fingerprints = %d, want 2: %+v", len(got), got)
	}
	// Oldest first — fp1 (op) was added before fp2 (ed).
	if got[0].Kind != domain.SkipKindOp || got[1].Kind != domain.SkipKindEd {
		t.Fatalf("order = %+v", got)
	}
	if len(got[0].Fp) != 4 || got[0].Fp[3] != 4294967295 {
		t.Fatalf("fp ints not preserved: %+v", got[0].Fp)
	}
	if got[0].ID == "" {
		t.Fatal("expected uuid to be filled")
	}
}
