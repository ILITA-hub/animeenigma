package embeds

import (
	"os"
	"path/filepath"
	"testing"
)

// vibePlayerGolden resolves the vibeplayer_embed.html golden captured in
// Plan 18-01 Task 3 (path: services/scraper/testdata/gogoanime/vibeplayer_embed.html).
// Used by the RED-state tests below to fail loudly if the goldens have not
// been captured yet.
func vibePlayerGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "vibeplayer_embed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestVibePlayer_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate: VibePlayerExtractor.Matches() must reject impostor hosts that
// merely contain "vibeplayer.site" as a substring (e.g. evilvibeplayer.site)
// — only host equality OR strict subdomain (HasSuffix host, "."+known) match.
func TestVibePlayer_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-03")
}

// TestVibePlayer_Extract_FromGolden verifies SCRAPER-9ANI-04: vibeplayer
// extractor parses `const src = "https://...m3u8"` from the captured
// vibeplayer_embed.html golden via regex (no goja — regex-only path).
func TestVibePlayer_Extract_FromGolden(t *testing.T) {
	_ = vibePlayerGolden(t)
	t.Skip("RED — implementation arrives in Plan 18-03")
}
