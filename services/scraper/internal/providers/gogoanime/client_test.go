package gogoanime

import (
	"os"
	"path/filepath"
	"testing"
)

// goldenPath resolves the path to a Phase 18 golden fixture (Plan 18-01 Task 3
// captured the 8 anitaku.to / embed-wrapper / malsync goldens under
// services/scraper/testdata/gogoanime/). Used by every RED-state test below to
// fail loudly if the goldens have not been captured yet.
func goldenPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "gogoanime", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden %s missing: %v", name, err)
	}
	return p
}

// TestFindID_FuzzyPath verifies SCRAPER-9ANI-01: fuzzy /search.html is the
// PRIMARY ID resolution path (malsync has no Gogoanime/Anitaku key as of
// 2026-05-12 — see services/scraper/testdata/gogoanime/malsync_no_gogo.json).
func TestFindID_FuzzyPath(t *testing.T) {
	t.Helper()
	goldenPath(t, "search_attack_on_titan.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestFindID_MalsyncNegativeCache verifies SCRAPER-9ANI-01: malsync miss is
// negative-cached for 24h (forward-compat probe stays in code so we get the
// fast path if/when malsync ships a Gogoanime/Anitaku key).
func TestFindID_MalsyncNegativeCache(t *testing.T) {
	t.Helper()
	goldenPath(t, "malsync_no_gogo.json")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestListEpisodes_SubDubMerge verifies SCRAPER-9ANI-02: ListEpisodes merges
// /category/<slug> (sub) + /category/<slug>-dub (dub) into one episode list,
// preserving the sub/dub split via the Server.Type derived from the URL slug.
func TestListEpisodes_SubDubMerge(t *testing.T) {
	t.Helper()
	goldenPath(t, "category_one_piece.html")
	goldenPath(t, "category_one_piece_dub.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestListEpisodes_CacheHit verifies SCRAPER-9ANI-02: episodes cached 6h
// at cache key episodes:gogoanime:<slug>.
func TestListEpisodes_CacheHit(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestListServers_AnimeMutiLink verifies SCRAPER-9ANI-03: ListServers parses
// <ul class="anime_muti_link"> on the episode page (HD-1, HD-2, StreamHG,
// Earnvids).
func TestListServers_AnimeMutiLink(t *testing.T) {
	t.Helper()
	goldenPath(t, "one_piece_episode_1.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestListServers_DoodstreamSkipped verifies SCRAPER-9ANI-03: Turnstile-gated
// embed hosts (myvidplay.com / playmogo.com) are filtered out at ListServers
// time — we never surface them to the orchestrator's GetStream path.
func TestListServers_DoodstreamSkipped(t *testing.T) {
	t.Helper()
	goldenPath(t, "one_piece_episode_1.html")
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestGetStream_DispatchesToRegistry verifies SCRAPER-9ANI-04: GetStream
// dispatches the matched embed URL to embeds.Registry.Find(serverID).Extract().
// No extraction logic lives inside the gogoanime client itself.
func TestGetStream_DispatchesToRegistry(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-02")
}

// TestGetStream_StreamTTL verifies SCRAPER-9ANI-04: stream TTL =
// min(parsedExpiry - 30s, 5min) per RESEARCH.md, with StreamHG/Earnvids
// using &e=<seconds_to_live> (delta) and Vibeplayer falling back to 5min
// (no expiry param).
func TestGetStream_StreamTTL(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-02")
}
