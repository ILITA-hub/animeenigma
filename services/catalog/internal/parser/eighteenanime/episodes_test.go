package eighteenanime

import (
	"os"
	"strings"
	"testing"
)

func TestParseEpisodeMirrors(t *testing.T) {
	data, err := os.ReadFile("testdata/episode_page.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	mirrors := parseEpisodeMirrors(string(data))

	if len(mirrors) < 1 {
		t.Fatalf("expected at least 1 mirror, got 0")
	}

	// At least one mirror should be an mp4upload or turbovid link.
	hasSupported := false
	for _, m := range mirrors {
		if strings.Contains(m.Link, "mp4upload") || strings.Contains(m.Link, "turbovid") {
			hasSupported = true
			break
		}
	}
	if !hasSupported {
		t.Errorf("no mp4upload or turbovid mirror found in %d mirrors", len(mirrors))
	}

	// Links must be deduplicated.
	seen := map[string]int{}
	for _, m := range mirrors {
		seen[m.Link]++
	}
	for link, count := range seen {
		if count > 1 {
			t.Errorf("duplicate link in output: %q (count=%d)", link, count)
		}
	}

	t.Logf("unique mirrors found: %d", len(mirrors))
	for _, m := range mirrors {
		t.Logf("  link=%s quality=%s", m.Link, m.Quality)
	}
}

func TestSupportedMirrors(t *testing.T) {
	all := []Mirror{
		{Link: "https://www.mp4upload.com/embed-abc.html", Quality: "FullHD"},
		{Link: "https://turbovidhls.com/t/xyz", Quality: "FullHD"},
		{Link: "https://abyssplayer.com/G97Y_rAZz", Quality: "FullHD"},
	}

	got := supportedMirrors(all)

	if len(got) != 2 {
		t.Fatalf("expected 2 supported mirrors, got %d", len(got))
	}

	// mp4upload must come first (failover order).
	if !strings.Contains(got[0].Link, "mp4upload") {
		t.Errorf("expected first mirror to be mp4upload, got %q", got[0].Link)
	}

	if !strings.Contains(got[1].Link, "turbovid") {
		t.Errorf("expected second mirror to be turbovid, got %q", got[1].Link)
	}

	// abyssplayer must be excluded.
	for _, m := range got {
		if strings.Contains(m.Link, "abyss") {
			t.Errorf("abyssplayer should be excluded but found: %q", m.Link)
		}
	}
}
