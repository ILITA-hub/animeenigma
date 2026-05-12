package fuzzy

import (
	"math"
	"testing"
)

// TestNormalizeTitle_Cases pins the season-suffix folding + punctuation
// collapse behaviour. Migrated from animepahe.cache_test.go::TestNormalizeTitle.
func TestNormalizeTitle_Cases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		// Migrated from animepahe coverage (regression-locked).
		{"Vinland Saga: 2nd Season", "vinland saga season 2"},
		{"Vinland Saga Part 2", "vinland saga season 2"},
		{"Naruto", "naruto"},
		{"Naruto: Shippuuden", "naruto shippuuden"},
		{"  Spaces  collapsed  ", "spaces collapsed"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTitle(c.in); got != c.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestJaroWinkler_KnownPairs verifies the Jaro-Winkler implementation against
// well-known reference values from the original paper, plus the bracketing
// cases used by the AnimePahe fuzzy fallback (threshold = 0.85).
//
// Migrated from animepahe.cache_test.go::TestJaroWinkler; all expected
// values come from the in-tree implementation's actual output (regression
// invariant — pure code motion must not change scores).
func TestJaroWinkler_KnownPairs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		// approx within 0.05 (matches animepahe test tolerance).
		want float64
	}{
		{"naruto", "naruto", 1.00},                      // exact
		{"naruto", "narutoo", 0.97},                     // off-by-one
		{"naruto", "naruto shippuuden", 0.88},           // prefix-match boost
		{"vinland saga", "vinland saga season 2", 0.92}, // shared prefix
		{"one piece", "two piece", 0.85},                // partial-rewrite
		{"naruto", "xxxxxxx", 0.00},                     // no common chars
		{"", "", 1.00},                                  // both empty: exact
		{"", "naruto", 0.00},                            // one empty
	}
	for _, c := range cases {
		c := c
		t.Run(c.a+"_vs_"+c.b, func(t *testing.T) {
			t.Parallel()
			got := JaroWinkler(c.a, c.b)
			if math.Abs(got-c.want) > 0.05 {
				t.Errorf("JaroWinkler(%q,%q) = %.4f; want ~%.4f", c.a, c.b, got, c.want)
			}
		})
	}
}
