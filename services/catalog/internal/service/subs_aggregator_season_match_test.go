package service

import "testing"

func TestSeasonMarkerIndex(t *testing.T) {
	cases := []struct {
		title string
		want  int
	}{
		{"Clevatess Season 2", 2},
		{"Clevatess II: Majuu no Ou to Itsuwari no Yuusha Denshou", 2},
		{"Sword Art Online II", 2},
		{"Kaguya-sama wa Kokurasetai 3", 3},
		{"Overlord IV", 4},
		{"07-Ghost", 1},        // mid-title number, not last/season-prefixed — no marker
		{"91 Days", 1},         // leading number, not last/season-prefixed — no marker
		{"Attack on Titan", 1}, // no marker at all
		{"Clevatess.S01E03.Heros.Duty.1080p.CR.WEB-DL.DUAL.AAC2.0.H.264-VARYG", 1},
		{"Clevatess II ~Majuu no Ou to Itsuwari no Yuusha Denshou~ - S01E01", 2},
	}
	for _, c := range cases {
		if got := seasonMarkerIndex(c.title); got != c.want {
			t.Errorf("seasonMarkerIndex(%q) = %d, want %d", c.title, got, c.want)
		}
	}
}

func TestTitleLikelyMatches(t *testing.T) {
	// Regression: the actual live-confirmed mismatch (2026-07-17, tNeymik
	// report) — "Clevatess Season 2" ep3 free-text search resolved to the
	// original (season-1) show's real episode 3.
	if titleLikelyMatches("Clevatess Season 2", "Clevatess.S01E03.Heros.Duty.1080p.CR.WEB-DL.DUAL.AAC2.0.H.264-VARYG") {
		t.Error("expected mismatch to be rejected: wrong-show release with no sequel marker")
	}
	// A correctly-tagged season-2 release must still pass.
	if !titleLikelyMatches("Clevatess Season 2", "Clevatess II ~Majuu no Ou to Itsuwari no Yuusha Denshou~ - S01E01") {
		t.Error("expected correctly-tagged sequel release to match")
	}
	// Ordinary single-season titles are never filtered.
	if !titleLikelyMatches("Attack on Titan", "Shingeki.no.Kyojin.S01E01.1080p.WEB-DL") {
		t.Error("expected single-season title to pass through unfiltered")
	}
	// A title with an incidental non-season number must not be broken by
	// the guard (07-Ghost has no marker, so nothing is required downstream).
	if !titleLikelyMatches("07-Ghost", "07.Ghost.S01E01.1080p.WEB-DL") {
		t.Error("expected 07-Ghost (no sequel marker) to pass through unfiltered")
	}
}
