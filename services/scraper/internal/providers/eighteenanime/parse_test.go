package eighteenanime

import (
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestExtractMP4Upload(t *testing.T) {
	src, err := extractMP4Upload(readFixture(t, "embed_mp4upload.html"))
	if err != nil {
		t.Fatalf("extractMP4Upload: %v", err)
	}
	if !strings.Contains(src.URL, "mp4upload.com") || !strings.Contains(src.URL, ".mp4") {
		t.Fatalf("unexpected URL: %s", src.URL)
	}
	if src.Referer != "https://www.mp4upload.com/" {
		t.Fatalf("expected mp4upload referer, got %q", src.Referer)
	}
	if src.IsHLS {
		t.Fatal("mp4upload should be MP4, not HLS")
	}
}

func TestExtractTurbovid(t *testing.T) {
	src, err := extractTurbovid(readFixture(t, "embed_turbovid.html"))
	if err != nil {
		t.Fatalf("extractTurbovid: %v", err)
	}
	if !strings.Contains(src.URL, ".m3u8") {
		t.Fatalf("expected m3u8, got %s", src.URL)
	}
	if !src.IsHLS {
		t.Fatal("turbovid should be HLS")
	}
}

func TestParseSearchResults(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_results.html"))
	if len(hits) == 0 {
		t.Fatal("expected >=1 hit")
	}
	seen := map[string]int{}
	for _, h := range hits {
		if h.Slug == "" || h.URL == "" {
			t.Fatalf("bad hit: %+v", h)
		}
		seen[h.Slug]++
	}
	for slug, n := range seen {
		if n > 1 {
			t.Fatalf("dup slug %q (%d)", slug, n)
		}
	}
}

func TestBestMatch(t *testing.T) {
	hits := parseSearchResults(readFixture(t, "search_results.html"))
	got := bestMatch("JK to Inkou Kyoushi 4", hits)
	if got == nil || !strings.Contains(got.Slug, "jk-to-inkou-kyoushi-4") {
		t.Fatalf("bestMatch wrong: %+v", got)
	}
	if bestMatch("totally unrelated xyzzy", hits) != nil {
		t.Fatal("expected nil for unrelated query")
	}
	// AUTO-630: a CJK-only title has no ASCII-representable form, so every
	// token normalizes to "". It must NOT false-match arbitrary unrelated
	// slugs via strings.Contains(slug, "") — bestMatch must return nil so the
	// caller falls through to no-content / the next alt title.
	unrelated := []SearchHit{
		{Slug: "1234-lamour-fou-de-lautomate", URL: "https://18anime.me/watch/1234-lamour-fou-de-lautomate"},
		{Slug: "5678-some-other-series", URL: "https://18anime.me/watch/5678-some-other-series"},
	}
	if got := bestMatch("恋する乙女", unrelated); got != nil {
		t.Fatalf("expected nil for CJK-only title against unrelated slugs, got %+v", got)
	}
	// AUTO-593: a lone stopword overlap ("the") must not create a false
	// match — live incident: "Imaizumi Brings All the Gyarus to His House"
	// matched an unrelated slug purely via its "-the-animation-" segment.
	stopwordOnly := []SearchHit{
		{Slug: "9999-reika-wa-karei-na-boku-no-joou-the-animation-4", URL: "https://18anime.me/hentai/9999-reika-wa-karei-na-boku-no-joou-the-animation-4.html"},
	}
	if got := bestMatch("Imaizumi Brings All the Gyarus to His House", stopwordOnly); got != nil {
		t.Fatalf("expected nil for stopword-only overlap, got %+v", got)
	}
}

func TestParseEpisodeMirrors(t *testing.T) {
	mirrors := parseEpisodeMirrors(readFixture(t, "episode_page.html"))
	if len(mirrors) == 0 {
		t.Fatal("expected >=1 mirror")
	}
	var mp4, turbo bool
	for _, m := range mirrors {
		if strings.Contains(m.Link, "mp4upload") {
			mp4 = true
		}
		if strings.Contains(m.Link, "turbovid") {
			turbo = true
		}
	}
	if !mp4 && !turbo {
		t.Fatal("expected mp4upload or turbovid mirror")
	}
}

func TestSupportedMirrorsOrder(t *testing.T) {
	all := []Mirror{
		{Link: "https://bysesayeveum.com/e/x"},
		{Link: "https://turbovidhls.com/t/x"},
		{Link: "https://www.mp4upload.com/embed-x.html"},
	}
	got := supportedMirrors(all)
	if len(got) != 2 || !strings.Contains(got[0].Link, "mp4upload") || !strings.Contains(got[1].Link, "turbovid") {
		t.Fatalf("supportedMirrors order wrong: %+v", got)
	}
}

func TestBaseSlugAndEpisode(t *testing.T) {
	cases := []struct {
		slug     string
		wantBase string
		wantNum  int
	}{
		{"1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2", "jk-to-inkou-kyoushi-4-feat-ero-giin-sensei", 2},
		{"1164-jk-to-inkou-kyoushi-4-episode-1", "jk-to-inkou-kyoushi-4", 1},
		{"2171-inkou-kyoushi-no-saimin-seikatsu-shidouroku-1", "inkou-kyoushi-no-saimin-seikatsu-shidouroku", 1},
	}
	for _, c := range cases {
		base, num := baseSlugAndEpisode(c.slug)
		if base != c.wantBase || num != c.wantNum {
			t.Fatalf("%s -> (%q,%d), want (%q,%d)", c.slug, base, num, c.wantBase, c.wantNum)
		}
	}
}
