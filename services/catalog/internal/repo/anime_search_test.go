package repo

// Unit tests for the search-query normalizer behind the punctuation-
// insensitive Search match (feedback 2026-06-11T12-53-52: "re zero" must
// find "Re:Zero"). The SQL side (regexp_replace on the name columns) is
// Postgres-only and verified against the live DB; these tests pin the Go
// side that must mirror it.

import "testing"

func TestNormalizeSearchQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain words collapse spaces", "re zero", "rezero"},
		{"punctuation stripped", "Re:Zero", "rezero"},
		{"slash stripped", "Fate/Zero", "fatezero"},
		{"uppercased latin lowered", "DEATH NOTE", "deathnote"},
		{"cyrillic kept and lowered", "Жизнь с нуля!", "жизньснуля"},
		{"japanese kept", "ゼロから始める異世界生活", "ゼロから始める異世界生活"},
		{"digits kept", "86 part 2", "86part2"},
		{"like wildcards stripped", "%_re%zero_%", "rezero"},
		{"only punctuation yields empty", ":-!?.", ""},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeSearchQuery(tc.in); got != tc.want {
				t.Errorf("normalizeSearchQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
