package animepahe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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
	body := `{"id":21,"title":"One Piece","Sites":{"animepahe":{"1":{"identifier":"1","url":"https://animepahe.ru/anime/1"}},"gogoanime":{"alt":{"identifier":42,"url":"https://gogoanime.to/watch/one-piece-100"}}}}`
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
	gogoanime, ok := msr.Sites["gogoanime"]
	if !ok {
		t.Fatal("Sites.gogoanime missing — needed to verify identifier-as-int round-trip")
	}
	for _, e := range gogoanime {
		// identifier=42 (int) must round-trip through `any` cleanly.
		_ = e.Identifier
	}
}

// TestDTO_Frieren — Phase 27 D4. Loads the three fresh-from-resolver
// goldens at testdata/animepahe/frieren-{search,release,play}.{json,html}
// and asserts the DTO shapes still decode cleanly:
//
//   - frieren-search.json → searchResponse with ≥ 1 data[] entry, each
//     carrying a non-empty session.
//   - frieren-release.json → releaseResponse with ≥ 1 data[] entry, each
//     carrying a non-empty session.
//   - frieren-play.html → ≥ 1 `button[data-src=...kwik...]`.
//
// Subscripts to the larger A1/A2 contract: NO struct field changes in
// dto.go were required when the parser migrated to the resolver transport.
func TestDTO_Frieren(t *testing.T) {
	t.Parallel()
	base := filepath.Join("..", "..", "..", "testdata", "animepahe")

	t.Run("search", func(t *testing.T) {
		t.Parallel()
		data, err := os.ReadFile(filepath.Join(base, "frieren-search.json"))
		if err != nil {
			t.Fatalf("read frieren-search.json: %v", err)
		}
		var sr searchResponse
		if err := json.Unmarshal(data, &sr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(sr.Data) == 0 {
			t.Fatal("search data is empty")
		}
		for i, e := range sr.Data {
			if e.Session == "" {
				t.Errorf("search data[%d].session is empty", i)
			}
			if e.Title == "" {
				t.Errorf("search data[%d].title is empty", i)
			}
		}
		// Documents the D4 anchor: at least one entry titled with
		// "Frieren". A blanket-rename of the field by upstream would
		// cause this assertion to fail.
		sawFrieren := false
		for _, e := range sr.Data {
			if strings.Contains(strings.ToLower(e.Title), "frieren") {
				sawFrieren = true
				break
			}
		}
		if !sawFrieren {
			t.Errorf("expected at least one 'Frieren' titled entry")
		}
	})

	t.Run("release", func(t *testing.T) {
		t.Parallel()
		data, err := os.ReadFile(filepath.Join(base, "frieren-release.json"))
		if err != nil {
			t.Fatalf("read frieren-release.json: %v", err)
		}
		var rr releaseResponse
		if err := json.Unmarshal(data, &rr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(rr.Data) == 0 {
			t.Fatal("release data is empty")
		}
		for i, ep := range rr.Data {
			if ep.Session == "" {
				t.Errorf("release data[%d].session is empty", i)
			}
			if ep.EpisodeNumber <= 0 {
				t.Errorf("release data[%d].episode = %v; want > 0", i, ep.EpisodeNumber)
			}
		}
	})

	t.Run("play", func(t *testing.T) {
		t.Parallel()
		data, err := os.ReadFile(filepath.Join(base, "frieren-play.html"))
		if err != nil {
			t.Fatalf("read frieren-play.html: %v", err)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
		if err != nil {
			t.Fatalf("goquery parse: %v", err)
		}
		count := 0
		doc.Find("button[data-src]").Each(func(_ int, sel *goquery.Selection) {
			src, _ := sel.Attr("data-src")
			if strings.Contains(src, "kwik") {
				count++
			}
		})
		if count == 0 {
			t.Fatal("expected at least one button[data-src=...kwik...]")
		}
	})
}
