package animejoy

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func hitByID(hits []searchHit, id string) (searchHit, bool) {
	for _, h := range hits {
		if h.NewsID == id {
			return h, true
		}
	}
	return searchHit{}, false
}

func TestParseSearchResultsFrieren(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_frieren.html"))

	// Must include both Frieren seasons, in the tv-serialy section.
	h1, ok := hitByID(hits, "3647")
	if !ok {
		t.Fatalf("missing news_id 3647; hits=%v", hits)
	}
	if h1.Section != "tv-serialy" {
		t.Fatalf("3647 section: want tv-serialy, got %q", h1.Section)
	}
	if h1.Title == "" || h1.Title == "Смотреть" {
		t.Fatalf("3647 has bad title %q", h1.Title)
	}
	if _, ok := hitByID(hits, "5449"); !ok {
		t.Fatalf("missing news_id 5449; hits=%v", hits)
	}

	// No duplicate news_ids (the "Смотреть" link must be deduped against the
	// title link).
	seen := map[string]int{}
	for _, h := range hits {
		seen[h.NewsID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("news_id %s appears %d times (dedupe failed)", id, n)
		}
	}

	// No empty / "Смотреть" titles survived, and no off-site (ajsub.online)
	// decoy leaked in.
	for _, h := range hits {
		if h.Title == "" || h.Title == "Смотреть" {
			t.Fatalf("hit %+v has placeholder title", h)
		}
		if h.NewsID == "5422" {
			t.Fatalf("off-site ajsub.online decoy 5422 leaked into hits")
		}
	}
}

func TestScoreAndPickFrierenSeasons(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_frieren.html"))

	s1 := Query{Titles: []string{"Frieren: Beyond Journey's End", "Провожающая в последний путь Фрирен", "Sousou no Frieren"}, Season: 1, Kind: "TV", Year: 2023}
	if got, ok := scoreAndPick(hits, s1); !ok || got != "3647" {
		t.Fatalf("Frieren S1: want 3647, got %q ok=%v", got, ok)
	}

	s2 := Query{Titles: []string{"Frieren: Beyond Journey's End", "Провожающая в последний путь Фрирен", "Sousou no Frieren"}, Season: 2, Kind: "TV", Year: 2026}
	if got, ok := scoreAndPick(hits, s2); !ok || got != "5449" {
		t.Fatalf("Frieren S2: want 5449, got %q ok=%v", got, ok)
	}
}

func TestScoreAndPickCodeGeassR2(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_codegeass.html"))

	// Code Geass R2 == season 2, TV. Must pick 1007, NOT the films
	// (1010/1951/1952/2417), NOT the OVA (1009), NOT S1 (1006), NOT the later
	// "Вернувшийся Розе" (4839).
	q := Query{
		Titles: []string{
			"Code Geass: Hangyaku no Lelouch R2",
			"Код Гиас: Восставший Лелуш",
		},
		Season: 2,
		Kind:   "TV",
		Year:   2008,
	}
	got, ok := scoreAndPick(hits, q)
	if !ok {
		t.Fatalf("Code Geass R2: no pick")
	}
	if got != "1007" {
		t.Fatalf("Code Geass R2: want 1007, got %q", got)
	}

	// And S1 resolves to 1006.
	q1 := q
	q1.Season = 1
	if got, ok := scoreAndPick(hits, q1); !ok || got != "1006" {
		t.Fatalf("Code Geass S1: want 1006, got %q ok=%v", got, ok)
	}
}

func TestScoreAndPickRejectsWrongKind(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_codegeass.html"))
	// A movie query should not pick a tv-serialy row; it should land on one of
	// the anime-films rows.
	q := Query{Titles: []string{"Код Гиас: Лелуш Воскресший"}, Kind: "Movie"}
	got, ok := scoreAndPick(hits, q)
	if !ok {
		t.Fatalf("movie query: no pick")
	}
	// Whatever it picks, it must be a film section row (the 2417 exact match is
	// the natural winner here).
	h, _ := hitByID(hits, got)
	if h.Section != "anime-films" {
		t.Fatalf("movie query picked non-film %q (section %q)", got, h.Section)
	}
}
