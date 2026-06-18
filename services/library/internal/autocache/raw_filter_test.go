package autocache

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// TestSelectRAWFilter is the table-driven TRIG-05 contract: selectRAW returns
// the first (best-seeded, since input is ranked DESC) release that is RAW,
// parses to a resolution ≤ qualityCap, and has Seeders ≥ minSeeders — and
// (found=false) when none qualify.
func TestSelectRAWFilter(t *testing.T) {
	const (
		qualityCap = 1080
		minSeeders = 3
	)

	cases := []struct {
		name      string
		releases  []domain.Release
		wantFound bool
		wantTitle string
	}{
		{
			name: "RAW pass via uploader allowlist",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [1080p]", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 10},
			},
			wantFound: true,
			wantTitle: "Some Anime - 01 [1080p]",
		},
		{
			// WR-04: an unknown uploader with NO positive raw token no longer
			// qualifies (fail-closed). Previously this passed by defaulting open.
			name: "unknown uploader without positive raw token is NOT raw (WR-04 fail-closed)",
			releases: []domain.Release{
				{Title: "Some Anime - 01 (1080p) [JP]", Uploader: "RandomGroup", Quality: "1080p", Seeders: 5},
			},
			wantFound: false,
		},
		{
			// WR-04: an unknown uploader WITH an explicit positive raw token does
			// qualify — lets a legitimate raw from an unrecognized group through.
			name: "unknown uploader with positive raw token qualifies",
			releases: []domain.Release{
				{Title: "Some Anime - 01 (1080p) RAW", Uploader: "RandomGroup", Quality: "1080p", Seeders: 5},
			},
			wantFound: true,
			wantTitle: "Some Anime - 01 (1080p) RAW",
		},
		{
			name: "DUB token rejected even with good seeders/quality",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [1080p][DUB]", Uploader: "DubGroup", Quality: "1080p", Seeders: 99},
			},
			wantFound: false,
		},
		{
			name: "dual-audio token rejected",
			releases: []domain.Release{
				{Title: "Some Anime - 01 1080p Dual-Audio", Uploader: "RandomGroup", Quality: "1080p", Seeders: 99},
			},
			wantFound: false,
		},
		{
			name: "hardsub token rejected",
			releases: []domain.Release{
				{Title: "Some Anime - 01 1080p HardSub", Uploader: "RandomGroup", Quality: "1080p", Seeders: 99},
			},
			wantFound: false,
		},
		{
			name: "seeder gate: 1080p with 2 seeders rejected when minSeeders=3",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [1080p]", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 2},
			},
			wantFound: false,
		},
		{
			name: "quality cap: 2160p rejected when qualityCap=1080",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [2160p]", Uploader: "Ohys-Raws", Quality: "2160p", Seeders: 50},
			},
			wantFound: false,
		},
		{
			name: "missing/unparseable quality rejected (conservative)",
			releases: []domain.Release{
				{Title: "Some Anime - 01", Uploader: "Ohys-Raws", Quality: "", Seeders: 50},
			},
			wantFound: false,
		},
		{
			name: "first qualifying of a ranked slice wins (skips disqualified leaders)",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [1080p] DUB", Uploader: "DubGroup", Quality: "1080p", Seeders: 100}, // dub → skip
				{Title: "Some Anime - 01 [2160p]", Uploader: "Ohys-Raws", Quality: "2160p", Seeders: 80},     // over cap → skip
				{Title: "Some Anime - 01 [1080p]", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 7},      // winner
				{Title: "Some Anime - 01 [720p]", Uploader: "Ohys-Raws", Quality: "720p", Seeders: 4},        // also valid but lower-ranked
			},
			wantFound: true,
			wantTitle: "Some Anime - 01 [1080p]",
		},
		{
			name:      "empty input → no winner",
			releases:  nil,
			wantFound: false,
		},
		{
			name: "720p under cap accepted",
			releases: []domain.Release{
				{Title: "Some Anime - 01 [720p]", Uploader: "SubsPlease", Quality: "720p", Seeders: 3},
			},
			wantFound: true,
			wantTitle: "Some Anime - 01 [720p]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// All fixtures here are episode 01 of "Some Anime" with no MAL-ID, so
			// the episode-exact + title-match guards pass; this table exercises the
			// RAW/quality/seeder gates. The episode + identity guards have their own
			// table below (TestSelectRAW_EpisodeAndIdentityGuards).
			got, found := selectRAW(tc.releases, qualityCap, minSeeders, 1, 0, []string{"Some Anime"})
			if found != tc.wantFound {
				t.Fatalf("selectRAW found = %v, want %v (got title %q)", found, tc.wantFound, got.Title)
			}
			if found && got.Title != tc.wantTitle {
				t.Fatalf("selectRAW title = %q, want %q", got.Title, tc.wantTitle)
			}
		})
	}
}

// TestSelectRAW_EpisodeAndIdentityGuards is the v4.1 false-match regression: the
// pick must be the EXACT wanted episode AND the right anime (MAL-ID if the release
// carries one, else a title match). These are the real-world fixtures that
// motivated the fix.
func TestSelectRAW_EpisodeAndIdentityGuards(t *testing.T) {
	const qualityCap, minSeeders = 1080, 3

	cases := []struct {
		name      string
		releases  []domain.Release
		wantEp    int
		malID     int
		titles    []string
		wantFound bool
		wantTitle string
	}{
		{
			name:      "wrong anime via keyword hit is rejected (Kanojo for Bookworm)",
			releases:  []domain.Release{{Title: "[SubsPlease] Kanojo, Okarishimasu - 59 (1080p)", Uploader: "SubsPlease", Quality: "1080p", Seeders: 200}},
			wantEp:    10,
			titles:    []string{"Honzuki no Gekokujou", "Ascendance of a Bookworm"},
			wantFound: false, // episode 59≠10 AND title mismatch
		},
		{
			name:      "right anime wrong episode is rejected (Witch Hat ep12 for ep11)",
			releases:  []domain.Release{{Title: "[SubsPlease] Tongari Boushi no Atelier - 12 (1080p)", Uploader: "SubsPlease", Quality: "1080p", Seeders: 2005}},
			wantEp:    11,
			titles:    []string{"Tongari Boushi no Atelier", "Witch Hat Atelier"},
			wantFound: false, // episode 12≠11
		},
		{
			name:      "correct anime + SxxExx episode via title match (Classroom CR rip)",
			releases:  []domain.Release{{Title: "Classroom of the Elite S04E12 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (CR)", Uploader: "Tsundere-Raws", Quality: "1080p", Seeders: 160}},
			wantEp:    12,
			titles:    []string{"Youkoso Jitsuryoku Shijou Shugi no Kyoushitsu e", "Classroom of the Elite 4th Season"},
			wantFound: true,
			wantTitle: "Classroom of the Elite S04E12 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (CR)",
		},
		{
			name:      "MAL-ID-verified release accepted regardless of title wording",
			releases:  []domain.Release{{Title: "Ohys-Raws SomethingObscure - 05 (1080p)", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 10, MALID: 59708}},
			wantEp:    5,
			malID:     59708,
			titles:    []string{"unrelated romaji"},
			wantFound: true,
			wantTitle: "Ohys-Raws SomethingObscure - 05 (1080p)",
		},
		{
			name:      "MAL-ID present but mismatched is rejected",
			releases:  []domain.Release{{Title: "Ohys-Raws Whatever - 05 (1080p)", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 10, MALID: 99999}},
			wantEp:    5,
			malID:     59708,
			titles:    []string{"Whatever"},
			wantFound: false, // MAL-ID present → trusted, and 99999≠59708 (title fallback NOT consulted)
		},
		{
			name:      "unparseable episode is rejected (episode-exact always)",
			releases:  []domain.Release{{Title: "Some Anime Batch Complete 1080p", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 50}},
			wantEp:    3,
			titles:    []string{"Some Anime"},
			wantFound: false,
		},
		{
			name: "skips false leader, takes the correct lower-ranked release",
			releases: []domain.Release{
				{Title: "[SubsPlease] Kanojo, Okarishimasu - 59 (1080p)", Uploader: "SubsPlease", Quality: "1080p", Seeders: 999}, // wrong anime+ep
				{Title: "[Ohys-Raws] Honzuki no Gekokujou - 10 (1080p)", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 12},    // correct
			},
			wantEp:    10,
			titles:    []string{"Honzuki no Gekokujou", "Ascendance of a Bookworm"},
			wantFound: true,
			wantTitle: "[Ohys-Raws] Honzuki no Gekokujou - 10 (1080p)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := selectRAW(tc.releases, qualityCap, minSeeders, tc.wantEp, tc.malID, tc.titles)
			if found != tc.wantFound {
				t.Fatalf("selectRAW found = %v, want %v (got title %q)", found, tc.wantFound, got.Title)
			}
			if found && got.Title != tc.wantTitle {
				t.Fatalf("selectRAW title = %q, want %q", got.Title, tc.wantTitle)
			}
		})
	}
}

// TestEpisodeOf pins the multi-pattern episode parser used by the episode-exact
// guard: SxxEyy, EP/Episode token, and the generic "- NN" form.
func TestEpisodeOf(t *testing.T) {
	cases := []struct {
		title  string
		want   int
		wantOK bool
	}{
		{"Classroom of the Elite S04E12 VOSTFR 1080p", 12, true},
		{"[SubsPlease] Tongari Boushi no Atelier - 12 (1080p)", 12, true},
		{"[Ohys-Raws] Some Show - 05 [1080p]", 5, true},
		{"Some Show Episode 7 [1080p]", 7, true},
		{"Some Show EP08 1080p", 8, true},
		{"Some Show - 09v2 (1080p)", 9, true},
		{"Some Show Batch 01-12 Complete 1080p", 0, false}, // ambiguous batch → no single ep
		{"Some Movie 1080p BD", 0, false},
	}
	for _, tc := range cases {
		got, ok := episodeOf(tc.title)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("episodeOf(%q) = (%d,%v), want (%d,%v)", tc.title, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestIsRAW pins the WR-04 RAW classifier policy (fail-closed for unknown
// uploaders): a negative token always disqualifies; otherwise the release is RAW
// only when there is a positive signal — an allowlisted uploader OR an explicit
// positive raw token. "unknown uploader + no negative + no positive" is NOT RAW.
// Case-insensitive both ways.
func TestIsRAW(t *testing.T) {
	cases := []struct {
		title    string
		uploader string
		want     bool
	}{
		{"Anything at all", "ohys-raws", true},    // allowlist (lowercased)
		{"Anything at all", "Leopard-Raws", true}, // allowlist
		// WR-04: unknown uploader, no positive raw token → fail closed.
		{"Anime 01 1080p", "RandomGroup", false},
		// WR-04: unknown uploader WITH a positive raw token → admitted.
		{"Anime 01 1080p RAW", "RandomGroup", true},
		{"Anime 01 [1080p] Leopard-Raws", "RandomGroup", true}, // "Raws" token in title
		{"Anime 01 1080p JP-Audio", "RandomGroup", true},
		// Negative tokens still disqualify regardless of uploader/positive token.
		{"Anime 01 1080p DUB", "RandomGroup", false},
		{"Anime 01 1080p eng dub", "RandomGroup", false},
		{"Anime 01 1080p multi audio", "RandomGroup", false},
		{"Anime 01 1080p multi-sub RAW", "RandomGroup", false}, // negative wins over positive
		{"Anime 01 1080p English Audio", "RandomGroup", false}, // WR-04 expanded denylist
		{"Anime 01 1080p BD-ENG", "RandomGroup", false},        // WR-04 expanded denylist
		{"Anime 01 1080p HARDSUB", "Erai-raws", false},         // negative token wins even for allowlisted uploader
	}
	for _, tc := range cases {
		if got := isRAW(tc.title, tc.uploader); got != tc.want {
			t.Errorf("isRAW(%q, %q) = %v, want %v", tc.title, tc.uploader, got, tc.want)
		}
	}
}

// TestResolutionOf pins the quality token parse.
func TestResolutionOf(t *testing.T) {
	cases := []struct {
		quality string
		wantN   int
		wantOK  bool
	}{
		{"1080p", 1080, true},
		{"720p", 720, true},
		{"2160p", 2160, true},
		{"480p", 480, true},
		{"WEB-DL 1080p", 1080, true},
		{"", 0, false},
		{"HEVC", 0, false},
		{"360p", 0, false}, // not in the recognized token set
	}
	for _, tc := range cases {
		n, ok := resolutionOf(tc.quality)
		if ok != tc.wantOK || n != tc.wantN {
			t.Errorf("resolutionOf(%q) = (%d,%v), want (%d,%v)", tc.quality, n, ok, tc.wantN, tc.wantOK)
		}
	}
}
