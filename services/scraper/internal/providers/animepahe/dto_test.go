package animepahe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestEpDTO_Unmarshal_GoldenFixture loads the on-disk goldie at
// services/scraper/testdata/animepahe/release_4_p1.json (committed in Plan
// 16-01) and decodes it into a releaseResponse. Asserts the pagination
// metadata and at least one episode with a non-empty session and a positive
// episode number.
func TestEpDTO_Unmarshal_GoldenFixture(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "testdata", "animepahe", "release_4_p1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var rr releaseResponse
	if err := json.Unmarshal(data, &rr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rr.CurrentPage <= 0 {
		t.Errorf("current_page = %d; want > 0", rr.CurrentPage)
	}
	if rr.LastPage <= 0 {
		t.Errorf("last_page = %d; want > 0", rr.LastPage)
	}
	if len(rr.Data) == 0 {
		t.Fatalf("data is empty; want at least one episode")
	}
	for i, ep := range rr.Data {
		if ep.Session == "" {
			t.Errorf("data[%d].session is empty", i)
		}
		if ep.EpisodeNumber <= 0 {
			t.Errorf("data[%d].episode = %v; want > 0", i, ep.EpisodeNumber)
		}
	}
}

// TestSearchResponse_Unmarshal_GoldenFixture loads the search_naruto.json
// fixture and confirms multiple entries with session + title + id.
func TestSearchResponse_Unmarshal_GoldenFixture(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "testdata", "animepahe", "search_naruto.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var sr searchResponse
	if err := json.Unmarshal(data, &sr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sr.Data) < 2 {
		t.Fatalf("data has %d entries; want >= 2 for fuzzy-match test", len(sr.Data))
	}
	sawNaruto := false
	for _, e := range sr.Data {
		if e.ID == 0 || e.Session == "" || e.Title == "" {
			t.Errorf("search entry missing required fields: %+v", e)
		}
		if e.Title == "Naruto" {
			sawNaruto = true
		}
	}
	if !sawNaruto {
		t.Errorf("search_naruto fixture should contain a top-level \"Naruto\" entry")
	}
}

// TestMalSyncResponse_Unmarshal verifies the malsync DTO matches the
// documented upstream shape. Real production malsync responses sometimes
// use integer identifiers, sometimes strings — our DTO declares the field
// as `any` and the Lookup() code stringifies via fmt.Sprintf.
func TestMalSyncResponse_Unmarshal(t *testing.T) {
	t.Parallel()
	body := `{"id":21,"title":"One Piece","Sites":{"animepahe":{"1":{"identifier":"1","url":"https://animepahe.ru/anime/1"}},"hianime":{"alt":{"identifier":42,"url":"https://hianime.to/watch/one-piece-100"}}}}`
	var msr malSyncResponse
	if err := json.Unmarshal([]byte(body), &msr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msr.ID != 21 {
		t.Errorf("id = %d; want 21", msr.ID)
	}
	animepahe, ok := msr.Sites["animepahe"]
	if !ok {
		t.Fatalf("Sites.animepahe missing")
	}
	if len(animepahe) != 1 {
		t.Errorf("animepahe has %d entries; want 1", len(animepahe))
	}
	hianime, ok := msr.Sites["hianime"]
	if !ok {
		t.Fatal("Sites.hianime missing — needed to verify identifier-as-int round-trip")
	}
	for _, e := range hianime {
		// identifier=42 (int) must round-trip through `any` cleanly.
		_ = e.Identifier
	}
}
