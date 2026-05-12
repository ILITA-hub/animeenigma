package gogoanime

import "testing"

// Goldens live under services/scraper/testdata/gogoanime/ — resolved by the
// goldenPath helper in client_test.go (same package).

// TestSearchResult_GoldenParse exercises the search-result row DTO against
// the captured search_attack_on_titan.html golden (Plan 18-01 Task 3).
// SCRAPER-9ANI-01: each <a href="/category/<slug>"> entry yields one
// searchResult with Slug + Title populated.
func TestSearchResult_GoldenParse(t *testing.T) {
	_ = goldenPath(t, "search_attack_on_titan.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestEpisodeRow_GoldenParse exercises the episode-row DTO against the
// captured category_one_piece.html golden. SCRAPER-9ANI-02: each
// <a href="/<slug>-episode-N"> yields one episodeRow with Number parsed
// from the trailing -episode-<N> path segment.
func TestEpisodeRow_GoldenParse(t *testing.T) {
	_ = goldenPath(t, "category_one_piece.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestServerRow_GoldenParse exercises the server-row DTO against the
// captured one_piece_episode_1.html golden. SCRAPER-9ANI-03: each
// <ul class="anime_muti_link"> <li><a data-video> yields one serverRow
// with Name (visible label) + EmbedURL (raw data-video, https:-prefixed
// when protocol-relative).
func TestServerRow_GoldenParse(t *testing.T) {
	_ = goldenPath(t, "one_piece_episode_1.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}
