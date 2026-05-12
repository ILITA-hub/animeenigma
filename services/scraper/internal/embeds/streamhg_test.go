package embeds

import (
	"os"
	"path/filepath"
	"testing"
)

// streamhgGolden resolves the streamhg_packed.html golden captured in Plan
// 18-01 Task 3 (path: services/scraper/testdata/gogoanime/streamhg_packed.html).
// Used by the RED-state tests below to fail loudly if the goldens have not
// been captured yet.
func streamhgGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "streamhg_packed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestStreamHG_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate: StreamHGExtractor.Matches() must reject impostor hosts that
// merely contain "otakuhg.site" as a substring — only host equality OR
// strict subdomain (HasSuffix host, "."+known) match.
func TestStreamHG_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-03")
}

// TestStreamHG_Extract_FromGolden verifies SCRAPER-9ANI-04: StreamHG
// extracts hls2 from the Dean-Edwards packed body in the captured
// streamhg_packed.html golden.
func TestStreamHG_Extract_FromGolden(t *testing.T) {
	_ = streamhgGolden(t)
	t.Skip("RED — implementation arrives in Plan 18-03")
}

// TestStreamHG_ComputeTTL verifies SCRAPER-9ANI-04: TTL = min(parsed
// &e=<seconds_to_live> minus 30s, 5min). StreamHG's e= param is a delta,
// not an absolute Unix timestamp.
func TestStreamHG_ComputeTTL(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-03")
}
