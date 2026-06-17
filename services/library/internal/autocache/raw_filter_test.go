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
			name: "RAW pass via no-negative-token title",
			releases: []domain.Release{
				{Title: "Some Anime - 01 (1080p) [JP]", Uploader: "RandomGroup", Quality: "1080p", Seeders: 5},
			},
			wantFound: true,
			wantTitle: "Some Anime - 01 (1080p) [JP]",
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
				{Title: "Some Anime - 01 [2160p]", Uploader: "Ohys-Raws", Quality: "2160p", Seeders: 80},      // over cap → skip
				{Title: "Some Anime - 01 [1080p]", Uploader: "Ohys-Raws", Quality: "1080p", Seeders: 7},       // winner
				{Title: "Some Anime - 01 [720p]", Uploader: "Ohys-Raws", Quality: "720p", Seeders: 4},         // also valid but lower-ranked
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
			got, found := selectRAW(tc.releases, qualityCap, minSeeders)
			if found != tc.wantFound {
				t.Fatalf("selectRAW found = %v, want %v (got title %q)", found, tc.wantFound, got.Title)
			}
			if found && got.Title != tc.wantTitle {
				t.Fatalf("selectRAW title = %q, want %q", got.Title, tc.wantTitle)
			}
		})
	}
}

// TestIsRAW pins the RAW classifier directly: uploader allowlist OR no negative
// token. Case-insensitive both ways.
func TestIsRAW(t *testing.T) {
	cases := []struct {
		title    string
		uploader string
		want     bool
	}{
		{"Anything at all", "ohys-raws", true},      // allowlist (lowercased)
		{"Anything at all", "Leopard-Raws", true},   // allowlist
		{"Anime 01 1080p", "RandomGroup", true},     // no negative token
		{"Anime 01 1080p DUB", "RandomGroup", false},
		{"Anime 01 1080p eng dub", "RandomGroup", false},
		{"Anime 01 1080p multi audio", "RandomGroup", false},
		{"Anime 01 1080p HARDSUB", "Erai-raws", false}, // negative token wins even for allowlisted uploader
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
