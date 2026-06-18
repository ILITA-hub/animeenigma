package kodik

import "testing"

func TestResultEpisodeCount_Precedence(t *testing.T) {
	if got := resultEpisodeCount(SearchResult{LastEpisode: 12, EpisodesCount: 5}); got != 12 {
		t.Errorf("LastEpisode wins: got %d, want 12", got)
	}
	if got := resultEpisodeCount(SearchResult{EpisodesCount: 5}); got != 5 {
		t.Errorf("EpisodesCount fallback: got %d, want 5", got)
	}
	if got := resultEpisodeCount(SearchResult{Type: "anime"}); got != 1 {
		t.Errorf("anime min-1 fallback: got %d, want 1", got)
	}
	if got := resultEpisodeCount(SearchResult{}); got != 0 {
		t.Errorf("empty non-anime: got %d, want 0", got)
	}
}

func TestMaxAnyTeamEpisode(t *testing.T) {
	results := []SearchResult{
		{Translation: &Translation{ID: 1}, LastEpisode: 7},
		{Translation: &Translation{ID: 2}, LastEpisode: 12}, // a different team is further ahead
		{Translation: &Translation{ID: 3}, EpisodesCount: 9},
	}
	if got := maxAnyTeamEpisode(results); got != 12 {
		t.Errorf("maxAnyTeamEpisode = %d, want 12", got)
	}
	if got := maxAnyTeamEpisode(nil); got != 0 {
		t.Errorf("maxAnyTeamEpisode(nil) = %d, want 0", got)
	}
}
