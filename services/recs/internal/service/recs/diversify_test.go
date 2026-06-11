package recs

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeAttrLoader implements attrLoader with a fixed map.
type fakeAttrLoader struct{ sets map[string]map[string]struct{} }

func (f *fakeAttrLoader) LoadAttrSets(_ context.Context, ids []string) (map[string]map[string]struct{}, error) {
	return f.sets, nil
}

func attrSet(keys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

func recsList(pairs ...any) []Recommendation {
	out := make([]Recommendation, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Recommendation{AnimeID: AnimeID(pairs[i].(string)), Final: pairs[i+1].(float64)})
	}
	return out
}

func TestDiversify_LambdaZeroIsIdentity(t *testing.T) {
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"a": attrSet("genre:1"), "b": attrSet("genre:1"), "c": attrSet("genre:2"),
	}})
	in := recsList("a", 0.9, "b", 0.8, "c", 0.7)
	got, err := d.Rerank(context.Background(), in, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range in {
		if got[i].AnimeID != in[i].AnimeID {
			t.Fatalf("lambda=0 must preserve order; pos %d = %s, want %s", i, got[i].AnimeID, in[i].AnimeID)
		}
	}
}

func TestDiversify_PrefersDiverseOverNearDuplicate(t *testing.T) {
	// b is a clone of a (same genre+studio); c differs but scores slightly
	// lower. With lambda=0.3, c must take position 2 over b.
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"a": attrSet("genre:1", "studio:x"), "b": attrSet("genre:1", "studio:x"), "c": attrSet("genre:2", "studio:y"),
	}})
	in := recsList("a", 0.90, "b", 0.85, "c", 0.80)
	got, _ := d.Rerank(context.Background(), in, "", 0.3)
	want := []AnimeID{"a", "c", "b"}
	for i, w := range want {
		if got[i].AnimeID != w {
			t.Fatalf("order = [%s %s %s], want [a c b]", got[0].AnimeID, got[1].AnimeID, got[2].AnimeID)
		}
	}
}

func TestDiversify_GenreSetHardCap(t *testing.T) {
	// five items with IDENTICAL genre sets; cap=3 → position 4 must be the
	// different item f even at lambda=0 (cap is independent of lambda).
	sets := map[string]map[string]struct{}{
		"a": attrSet("genre:1"), "b": attrSet("genre:1"), "c": attrSet("genre:1"),
		"d": attrSet("genre:1"), "e": attrSet("genre:1"), "f": attrSet("genre:2"),
	}
	d := NewDiversifier(&fakeAttrLoader{sets: sets})
	in := recsList("a", 0.9, "b", 0.89, "c", 0.88, "d", 0.87, "e", 0.86, "f", 0.5)
	got, _ := d.Rerank(context.Background(), in, "", 0)
	if got[3].AnimeID != "f" {
		t.Fatalf("position 4 = %s, want f (cap of 3 identical genre-sets)", got[3].AnimeID)
	}
	if len(got) != len(in) {
		t.Fatalf("rerank must keep all items, got %d of %d", len(got), len(in))
	}
}

func TestDiversify_SeedCountsAsPicked(t *testing.T) {
	// seed "p" is a clone of "a": with the seed given, "a" takes the
	// similarity penalty immediately and diverse "c" wins position 1.
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"p": attrSet("genre:1", "studio:x"), "a": attrSet("genre:1", "studio:x"), "c": attrSet("genre:2", "studio:y"),
	}})
	in := recsList("a", 0.9, "c", 0.85)
	got, _ := d.Rerank(context.Background(), in, "p", 0.3)
	if got[0].AnimeID != "c" {
		t.Fatalf("seed-similar item must be demoted; pos 0 = %s, want c", got[0].AnimeID)
	}
	if len(got) != 2 {
		t.Fatalf("seed must not appear in output; len = %d", len(got))
	}
}

func TestDiversify_EmptyAndSingle(t *testing.T) {
	// nil/empty/1-item inputs return as-is, no loader panic.
	// Use a nil-map fake — LoadAttrSets must never be called for these cases.
	d := NewDiversifier(&fakeAttrLoader{sets: nil})

	// nil input
	got, err := d.Rerank(context.Background(), nil, "", 0.3)
	if err != nil {
		t.Fatalf("nil input: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("nil input: got %v, want nil", got)
	}

	// empty slice
	got, err = d.Rerank(context.Background(), []Recommendation{}, "", 0.3)
	if err != nil {
		t.Fatalf("empty input: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty input: got len %d, want 0", len(got))
	}

	// single item — loader IS called (len=1 falls through to the general path
	// only when we decide so; the spec says <=1 returns as-is without loading)
	single := recsList("a", 0.9)
	got, err = d.Rerank(context.Background(), single, "", 0.3)
	if err != nil {
		t.Fatalf("single input: unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].AnimeID != "a" {
		t.Fatalf("single input: got %v, want [{a 0.9}]", got)
	}
}

func TestDiversify_AllCappedRelaxes(t *testing.T) {
	// 4 items ALL sharing the same genre-set; cap=3 → 3 are picked normally,
	// 4th must still be returned (relax path). All 4 items must be in output.
	sets := map[string]map[string]struct{}{
		"a": attrSet("genre:1"), "b": attrSet("genre:1"),
		"c": attrSet("genre:1"), "d": attrSet("genre:1"),
	}
	d := NewDiversifier(&fakeAttrLoader{sets: sets})
	in := recsList("a", 0.9, "b", 0.8, "c", 0.7, "d", 0.6)
	got, err := d.Rerank(context.Background(), in, "", 0.2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("relax path must return all 4 items, got %d", len(got))
	}
	// First 3 should be a, b, c (highest scores, before relax kicks in);
	// 4th must be d (the remaining one after relax).
	if got[3].AnimeID != "d" {
		t.Fatalf("4th position = %s, want d (lowest-MMR item after relax)", got[3].AnimeID)
	}
}

// setupDiversifyTestDB creates a minimal in-memory SQLite DB for GormAttrLoader
// integration tests. Only anime_genres and anime_studios tables are needed.
func setupDiversifyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`).Error; err != nil {
		t.Fatalf("create anime_genres: %v", err)
	}
	if err := db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT NOT NULL,
		studio_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, studio_id)
	)`).Error; err != nil {
		t.Fatalf("create anime_studios: %v", err)
	}
	return db
}

func TestGormAttrLoader_LoadsGenresAndStudios(t *testing.T) {
	db := setupDiversifyTestDB(t)

	// Seed: anime-1 has genre:action + studio:mappa; anime-2 has genre:comedy only.
	if err := db.Exec(`INSERT INTO anime_genres VALUES (?, ?)`, "anime-1", "action").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO anime_studios VALUES (?, ?)`, "anime-1", "mappa").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO anime_genres VALUES (?, ?)`, "anime-2", "comedy").Error; err != nil {
		t.Fatal(err)
	}

	loader := NewGormAttrLoader(db)
	sets, err := loader.LoadAttrSets(context.Background(), []string{"anime-1", "anime-2"})
	if err != nil {
		t.Fatalf("LoadAttrSets: %v", err)
	}

	// anime-1 must have genre:action and studio:mappa
	if _, ok := sets["anime-1"]["genre:action"]; !ok {
		t.Error("anime-1 missing genre:action")
	}
	if _, ok := sets["anime-1"]["studio:mappa"]; !ok {
		t.Error("anime-1 missing studio:mappa")
	}
	// anime-2 must have genre:comedy but no studio
	if _, ok := sets["anime-2"]["genre:comedy"]; !ok {
		t.Error("anime-2 missing genre:comedy")
	}
	if len(sets["anime-2"]) != 1 {
		t.Errorf("anime-2: want 1 attr, got %d", len(sets["anime-2"]))
	}
}

func TestGormAttrLoader_EmptyIDsReturnsEmpty(t *testing.T) {
	db := setupDiversifyTestDB(t)
	loader := NewGormAttrLoader(db)
	sets, err := loader.LoadAttrSets(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("want empty map, got %d entries", len(sets))
	}
}

func TestGormAttrLoader_UnknownIDReturnsNoEntry(t *testing.T) {
	db := setupDiversifyTestDB(t)
	loader := NewGormAttrLoader(db)
	sets, err := loader.LoadAttrSets(context.Background(), []string{"does-not-exist"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("unknown ID must produce no entry; got %d", len(sets))
	}
}
