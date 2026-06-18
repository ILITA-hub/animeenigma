package animelib

import "testing"

func TestMaxAnimeLibEpisode(t *testing.T) {
	eps := []Episode{{Number: "1"}, {Number: "12"}, {Number: "7"}}
	if got := maxAnimeLibEpisode(eps); got != 12 {
		t.Errorf("maxAnimeLibEpisode = %d, want 12", got)
	}
	// Non-numeric / fractional numbers are skipped, not fatal.
	if got := maxAnimeLibEpisode([]Episode{{Number: "3"}, {Number: "3.5"}, {Number: "abc"}}); got != 3 {
		t.Errorf("maxAnimeLibEpisode (mixed) = %d, want 3", got)
	}
	if got := maxAnimeLibEpisode(nil); got != 0 {
		t.Errorf("maxAnimeLibEpisode(nil) = %d, want 0", got)
	}
}
