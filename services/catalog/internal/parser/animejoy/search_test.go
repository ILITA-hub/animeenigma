package animejoy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSeasonExported(t *testing.T) {
	cases := map[string]int{
		"Re:Zero (2 сезон)":           2,
		"Frieren":                     1, // no marker → MVP default of 1
		"Attack on Titan Season 3":    3,
		"Some Show 2nd Season":        2,
		"Восставший… 1 сезон [25/25]": 1,
	}
	for title, want := range cases {
		if got := DetectSeason(title); got != want {
			t.Errorf("DetectSeason(%q) = %d, want %d", title, got, want)
		}
	}
}

// A standalone Latin roman numeral (II–IX) is a season marker even when it sits
// mid-title before a subtitle colon — the exact shape of the report
// 2026-07-09T06-40-52 catalog name "Mushoku Tensei III: Isekai Ittara Honki
// Dasu", which must resolve to season 3 (was silently defaulting to 1).
func TestDetectSeasonRomanNumeral(t *testing.T) {
	cases := map[string]int{
		"Mushoku Tensei III: Isekai Ittara Honki Dasu": 3, // roman before a colon
		"Overlord IV":                     4, // trailing roman
		"Kaguya-sama wa Kokurasetai II":   2,
		"Vinland Saga Part IX":            9,
		"無職転生 III ～異世界行ったら本気だす～": 3, // roman between CJK, space-delimited
	}
	for title, want := range cases {
		if got := DetectSeason(title); got != want {
			t.Errorf("DetectSeason(%q) = %d, want %d", title, got, want)
		}
	}
}

// A bare trailing single digit 2–9 (the AnimeJoy/Shikimori RU form, e.g.
// "…в другом мире 3") is a season marker. Multi-digit trailing numbers, a bare
// "0", or a title that is itself a number must NOT be misread as a season.
func TestDetectSeasonTrailingDigit(t *testing.T) {
	cases := map[string]int{
		"Реинкарнация безработного: История о приключениях в другом мире 3": 3,
		"Vinland Saga Part 2": 2,
		"Steins;Gate 0":       1, // 0 is not a season → default
		"Mob Psycho 100":      1, // multi-digit trailing = part of title
		"86":                  1, // the title IS a number
	}
	for title, want := range cases {
		if got := DetectSeason(title); got != want {
			t.Errorf("DetectSeason(%q) = %d, want %d", title, got, want)
		}
	}
}

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

// End-to-end regression for report 2026-07-09T06-40-52: AnimeJoy's Mushoku
// Tensei S3 row (news_id 5600) publishes its title in Latin/Cyrillic homoglyphs
// ("Peинкapнaция…"). Before foldConfusables the top (and only) hit scored 0.75
// against the clean catalog title, so scoreAndPick returned no match, the
// playlist came back empty, and both AllVideo + Sibnet families rendered
// no_content. With the fold it must resolve to 5600.
func TestScoreAndPickHomoglyphTitle(t *testing.T) {
	hits := []searchHit{{
		NewsID:  "5600",
		Title:   homoglyphify("Реинкарнация безработного: История о приключениях в другом мире (3 сезон) [02 из ХХ]"),
		Section: "tv-serialy",
	}}
	q := Query{
		Titles: []string{
			"Mushoku Tensei III: Isekai Ittara Honki Dasu",
			"Mushoku Tensei: Jobless Reincarnation Season 3",
			"Реинкарнация безработного: История о приключениях в другом мире 3",
		},
		Season: 3,
		Kind:   "TV",
		Year:   2026,
	}
	if got, ok := scoreAndPick(hits, q); !ok || got != "5600" {
		t.Fatalf("homoglyph S3: want 5600, got %q ok=%v", got, ok)
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
