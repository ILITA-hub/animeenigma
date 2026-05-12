package embeds

import (
	"os"
	"path/filepath"
	"testing"
)

// earnvidsGolden resolves the earnvids_packed.html golden captured in Plan
// 18-01 Task 3 (path: services/scraper/testdata/gogoanime/earnvids_packed.html).
// Used by the RED-state tests below to fail loudly if the goldens have not
// been captured yet.
func earnvidsGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "earnvids_packed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestEarnvids_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate: EarnvidsExtractor.Matches() must reject impostor hosts that
// merely contain "otakuvid.online" as a substring — only host equality OR
// strict subdomain (HasSuffix host, "."+known) match.
func TestEarnvids_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-03")
}

// TestEarnvids_Extract_FromGolden verifies SCRAPER-9ANI-04: Earnvids
// shares the Dean-Edwards packer unpack flow with StreamHG; differs only
// by host allowlist (otakuvid.online) and CDN (dramiyos-cdn.com instead
// of premilkyway.com).
func TestEarnvids_Extract_FromGolden(t *testing.T) {
	_ = earnvidsGolden(t)
	t.Skip("RED — implementation arrives in Plan 18-03")
}
