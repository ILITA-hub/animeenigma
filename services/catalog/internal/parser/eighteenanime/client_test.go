package eighteenanime

import (
	"os"
	"testing"
)

func TestParseSearchResults(t *testing.T) {
	data, err := os.ReadFile("testdata/search_results.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	hits := parseSearchResults(string(data))

	if len(hits) < 1 {
		t.Fatalf("expected at least 1 hit, got 0")
	}

	// Every hit must have non-empty Slug and URL.
	for i, h := range hits {
		if h.Slug == "" {
			t.Errorf("hits[%d].Slug is empty", i)
		}
		if h.URL == "" {
			t.Errorf("hits[%d].URL is empty", i)
		}
	}

	// Slugs must be deduplicated.
	seen := map[string]int{}
	for _, h := range hits {
		seen[h.Slug]++
	}
	for slug, count := range seen {
		if count > 1 {
			t.Errorf("slug %q appears %d times — expected exactly 1 (dedup failed)", slug, count)
		}
	}

	t.Logf("parseSearchResults found %d unique hits in fixture", len(hits))
}

func TestBestMatch(t *testing.T) {
	data, err := os.ReadFile("testdata/search_results.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	hits := parseSearchResults(string(data))

	got := bestMatch("JK to Inkou Kyoushi 4", hits)
	if got == nil {
		t.Fatal("bestMatch returned nil, want a hit containing 'jk-to-inkou-kyoushi-4'")
	}
	// The slug must contain the key title fragment.
	const wantFragment = "jk-to-inkou-kyoushi-4"
	slug := got.Slug
	found := false
	for i := 0; i <= len(slug)-len(wantFragment); i++ {
		if slug[i:i+len(wantFragment)] == wantFragment {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("bestMatch Slug = %q, want it to contain %q", slug, wantFragment)
	}

	t.Logf("bestMatch returned Slug=%q URL=%q", got.Slug, got.URL)
}

func TestBestMatch_NoMatch(t *testing.T) {
	data, err := os.ReadFile("testdata/search_results.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	hits := parseSearchResults(string(data))

	got := bestMatch("totally unrelated xyzzy", hits)
	if got != nil {
		t.Errorf("bestMatch returned non-nil hit for unrelated query: Slug=%q", got.Slug)
	}
}
