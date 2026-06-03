package videoutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDomainAllowed_EmptyList(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{},
	})

	assert.False(t, proxy.isDomainAllowed("example.com"), "empty AllowedDomains should fail-closed")
}

func TestIsDomainAllowed_NilList(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: nil,
	})

	assert.False(t, proxy.isDomainAllowed("example.com"), "nil AllowedDomains should fail-closed")
}

func TestIsDomainAllowed_ExactMatch(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com", "trusted.org"},
	})

	assert.True(t, proxy.isDomainAllowed("example.com"))
	assert.True(t, proxy.isDomainAllowed("trusted.org"))
}

func TestIsDomainAllowed_SubdomainMatch(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com"},
	})

	assert.True(t, proxy.isDomainAllowed("cdn.example.com"))
	assert.True(t, proxy.isDomainAllowed("a.b.c.example.com"))
}

func TestIsDomainAllowed_DisallowedDomain(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com", "trusted.org"},
	})

	assert.False(t, proxy.isDomainAllowed("evil.com"))
	assert.False(t, proxy.isDomainAllowed("malicious.org"))
	assert.False(t, proxy.isDomainAllowed("notexample.com"), "should not match suffix without dot separator")
}

func TestIsDomainAllowed_CaseInsensitive(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"Example.COM"},
	})

	assert.True(t, proxy.isDomainAllowed("example.com"))
	assert.True(t, proxy.isDomainAllowed("EXAMPLE.COM"))
	assert.True(t, proxy.isDomainAllowed("cdn.Example.Com"))
}

func TestIsHLSDomainAllowed_KnownDomains(t *testing.T) {
	knownDomains := []string{
		"megacloud.tv",
		"netmagcdn.com",
		"rapid-cloud.co",
		"jimaku.cc",
		"cdnlibs.org",
		"mcloud.to",
	}

	for _, domain := range knownDomains {
		assert.True(t, isHLSDomainAllowed(domain), "known domain %q should be allowed", domain)
	}
}

func TestIsHLSDomainAllowed_KnownSubdomains(t *testing.T) {
	assert.True(t, isHLSDomainAllowed("cdn.megacloud.tv"))
	assert.True(t, isHLSDomainAllowed("s1.netmagcdn.com"))
	assert.True(t, isHLSDomainAllowed("files.jimaku.cc"))
}

func TestIsHLSDomainAllowed_UnknownDomain(t *testing.T) {
	unknownDomains := []string{
		"evil.live",
		"random.pro",
		"hacker.xyz",
		"malware.com",
		"phishing.net",
	}

	for _, domain := range unknownDomains {
		assert.False(t, isHLSDomainAllowed(domain), "unknown domain %q should not be allowed", domain)
	}
}

func TestIsHLSDomainAllowed_PortStripping(t *testing.T) {
	assert.True(t, isHLSDomainAllowed("megacloud.tv:443"), "should strip port and match domain")
	assert.True(t, isHLSDomainAllowed("netmagcdn.com:8080"), "should strip port and match domain")
	assert.False(t, isHLSDomainAllowed("evil.com:443"), "should strip port but still reject unknown domain")
}

// TestHLSProxyAllowedDomainsList_MatchesProvenanceProjection verifies the
// flat []string view of the allow-list is exactly the Domain projection of
// the structured HLSProxyAllowedDomainsWithProvenance slice. This is the
// load-bearing invariant for the structured-provenance refactor: any caller
// that historically iterated HLSProxyAllowedDomains as []string must see the
// same entries in the same order after the refactor.
func TestHLSProxyAllowedDomainsList_MatchesProvenanceProjection(t *testing.T) {
	list := HLSProxyAllowedDomainsList()
	if len(list) != len(HLSProxyAllowedDomainsWithProvenance) {
		t.Fatalf("list length %d != provenance length %d", len(list), len(HLSProxyAllowedDomainsWithProvenance))
	}
	for i, e := range HLSProxyAllowedDomainsWithProvenance {
		if list[i] != e.Domain {
			t.Errorf("index %d: list=%q, provenance.Domain=%q", i, list[i], e.Domain)
		}
	}
	// Also lock the package-level HLSProxyAllowedDomains view (initialized
	// from HLSProxyAllowedDomainsList) to the same projection.
	if len(HLSProxyAllowedDomains) != len(HLSProxyAllowedDomainsWithProvenance) {
		t.Fatalf("HLSProxyAllowedDomains length %d != provenance length %d",
			len(HLSProxyAllowedDomains), len(HLSProxyAllowedDomainsWithProvenance))
	}
	for i, d := range HLSProxyAllowedDomains {
		if d != HLSProxyAllowedDomainsWithProvenance[i].Domain {
			t.Errorf("HLSProxyAllowedDomains[%d] = %q, want %q",
				i, d, HLSProxyAllowedDomainsWithProvenance[i].Domain)
		}
	}
}

// TestHLSProxyAllowedDomainsWithProvenance_HasNonEmptyMetadata verifies every
// entry carries Reason/Owner/Added — the audit script (scripts/audit-hls-allowlist.sh)
// relies on these fields being populated. Entries with empty provenance
// would silently print blank columns and defeat the quarterly review.
func TestHLSProxyAllowedDomainsWithProvenance_HasNonEmptyMetadata(t *testing.T) {
	for _, e := range HLSProxyAllowedDomainsWithProvenance {
		if e.Domain == "" {
			t.Errorf("entry has empty Domain: %+v", e)
		}
		if e.Reason == "" {
			t.Errorf("entry %q has empty Reason", e.Domain)
		}
		if e.Owner == "" {
			t.Errorf("entry %q has empty Owner", e.Domain)
		}
		if e.Added == "" {
			t.Errorf("entry %q has empty Added date", e.Domain)
		}
	}
}

// TestHLSProxyAllowedDomains_HasAnimePaheHosts locks the three AnimePahe CDN
// hosts in HLSProxyAllowedDomains. Without these, the HLS proxy returns 403
// for every AnimePahe stream → user-visible breakage. This is a
// regression-lock per SCRAPER-PAHE-05; the hosts are already present in
// libs/videoutils/proxy.go from prior work — this test PREVENTS a future
// PR from accidentally removing them.
func TestHLSProxyAllowedDomains_HasAnimePaheHosts(t *testing.T) {
	required := []string{"kwik.cx", "owocdn.top", "uwucdn.top"}
	have := make(map[string]bool, len(HLSProxyAllowedDomains))
	for _, d := range HLSProxyAllowedDomains {
		have[d] = true
	}
	for _, host := range required {
		if !have[host] {
			t.Errorf("HLSProxyAllowedDomains missing AnimePahe CDN host %q (required by SCRAPER-PAHE-05)", host)
		}
	}
}

// TestHLSProxyAllowedDomains_Phase18Additions locks the 5 new Anitaku/Gogoanime
// CDN hosts appended in Phase 18 Plan 18-04 Task 2. Missing entries cause the
// HLS proxy to 403 every stream coming through the gogoanime.Provider failover
// chain — user-visible breakage. Append-only edit: existing Phase 16 entries
// are protected by TestHLSProxyAllowedDomains_Phase16RegressionLocked below.
func TestHLSProxyAllowedDomains_Phase18Additions(t *testing.T) {
	want := []string{
		"anitaku.to",
		"vibeplayer.site",
		"premilkyway.com",
		"dramiyos-cdn.com",
		"cdn.cimovix.store",
	}
	present := make(map[string]bool, len(HLSProxyAllowedDomains))
	for _, d := range HLSProxyAllowedDomains {
		present[d] = true
	}
	for _, d := range want {
		if !present[d] {
			t.Errorf("HLSProxyAllowedDomains missing %q (Phase 18 addition)", d)
		}
	}
}

// TestHLSProxyAllowedDomains_Phase16RegressionLocked is the append-only
// invariant guard: every Phase 16 entry (and the jimaku.cc Phase 14 entry that
// shipped alongside) MUST still be present after the Phase 18 edit. Catches a
// regression where a future maintainer accidentally clobbers the slice
// declaration during an "append" edit.
func TestHLSProxyAllowedDomains_Phase16RegressionLocked(t *testing.T) {
	// kwik.cx + owocdn.top + uwucdn.top are AnimePahe CDN hosts (SCRAPER-PAHE-05).
	// jimaku.cc carries the Japanese subtitle files (Phase 14).
	want := []string{"kwik.cx", "owocdn.top", "uwucdn.top", "jimaku.cc"}
	present := make(map[string]bool, len(HLSProxyAllowedDomains))
	for _, d := range HLSProxyAllowedDomains {
		present[d] = true
	}
	for _, d := range want {
		if !present[d] {
			t.Errorf("Phase 16/14 entry %q is missing — append-only invariant broken", d)
		}
	}
}

// TestHLSProxyAllowedDomains_HasStreamhgHls3Hosts locks the two hls3 CDN
// hosts added in Phase 22 / SCRAPER-HEAL-10. Without these the multi-URL
// fallback shipped in Plan 22-01 returns URLs the streaming service refuses
// to proxy → user-visible breakage when hls2 signed URLs expire. This is a
// regression-lock that PREVENTS a future PR from accidentally removing them.
func TestHLSProxyAllowedDomains_HasStreamhgHls3Hosts(t *testing.T) {
	required := []string{"managementadvisory.sbs", "exoplanethunting.space"}
	have := make(map[string]bool, len(HLSProxyAllowedDomains))
	for _, d := range HLSProxyAllowedDomains {
		have[d] = true
	}
	for _, host := range required {
		if !have[host] {
			t.Errorf("HLSProxyAllowedDomains missing hls3 CDN host %q (required by SCRAPER-HEAL-10)", host)
		}
	}
}

// TestIsHLSDomainAllowed_Hls3Hosts exercises the gate behavior for the
// Phase 22 hls3 CDN hosts. Subdomain match (HasSuffix on "."+allowed),
// exact match, port-stripping, and the impostor-rejection contract are
// all pinned. Threat T-22-06 (SSRF expansion) is mitigated by the leading
// dot in the HasSuffix rule: `evilmanagementadvisory.sbs` does not match
// `managementadvisory.sbs`.
func TestIsHLSDomainAllowed_Hls3Hosts(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"managementadvisory.sbs", true},
		{"cdn.managementadvisory.sbs", true},
		{"a.b.managementadvisory.sbs", true},
		{"cdn.managementadvisory.sbs:443", true},
		{"managementadvisory.com", false},
		{"evilmanagementadvisory.sbs", false},
		{"managementadvisory.sbs.attacker.com", false},
		{"exoplanethunting.space", true},
		{"x.exoplanethunting.space", true},
		{"exoplanethunting.org", false},
		{"exoplanethunting.space:8080", true},
	}
	for _, c := range cases {
		if got := isHLSDomainAllowed(c.host); got != c.want {
			t.Errorf("isHLSDomainAllowed(%q) = %v; want %v", c.host, got, c.want)
		}
	}
}

// TestSolodcdnAllowed locks in the two solodcdn.com entries required by the
// Kodik ad-free HLS player (kodikextract). The manifest lives on
// cloud.solodcdn.com and 302-redirects to node subdomains such as
// draco.cloud.solodcdn.com — the eTLD+1 entry "solodcdn.com" covers those via
// the strings.HasSuffix(host, "."+allowed) gate in isHLSDomainAllowed.
// Hosts are extracted from the target URLs (isHLSDomainAllowed receives Host,
// not the full URL, matching how ProxyWithReferer calls it):
//   - https://cloud.solodcdn.com/useruploads/x/y:1/720.mp4:hls:manifest.m3u8
//   - https://draco.cloud.solodcdn.com/useruploads/x/y:1/720.mp4:hls:seg-1-v1-a1.ts
func TestSolodcdnAllowed(t *testing.T) {
	cases := []string{
		"cloud.solodcdn.com",       // manifest host (from .m3u8 URL above)
		"draco.cloud.solodcdn.com", // node subdomain (from .ts segment URL above)
	}
	for _, u := range cases {
		if !isHLSDomainAllowed(u) {
			t.Errorf("expected %s to be allowed", u)
		}
	}
}

// TestIsHLSDomainAllowed_RotatingSubdomains exercises the rotating-subdomain
// match policy needed by the new StreamHG + Earnvids CDNs (e.g.
// OkqtSs1gBbNcA8e.premilkyway.com per segment fetch). The existing
// strings.HasSuffix(host, "."+allowed) gate is the SSRF defense — the leading
// dot prevents impostor TLDs like evilanitaku.to from matching anitaku.to.
func TestIsHLSDomainAllowed_RotatingSubdomains(t *testing.T) {
	cases := []struct {
		host  string
		want  bool
		label string
	}{
		{"anitaku.to", true, "Phase 18 exact"},
		{"sub.anitaku.to", true, "Phase 18 subdomain"},
		{"OkqtSs1gBbNcA8e.premilkyway.com", true, "StreamHG rotating subdomain"},
		{"pfabiWMFmEza.dramiyos-cdn.com", true, "Earnvids rotating subdomain"},
		{"vibeplayer.site", true, "vibeplayer exact"},
		{"cdn.cimovix.store", true, "subtitle host exact"},
		{"evilanitaku.to", false, "impostor — no leading dot"},
		{"premilkyway.com.evil.com", false, "TLD impostor"},
		{"pacha.kwik.cx", true, "Phase 16 invariant — kwik subdomain"},
		{"jimaku.cc", true, "Phase 14 invariant — jimaku exact"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.label, func(t *testing.T) {
			if got := isHLSDomainAllowed(c.host); got != c.want {
				t.Errorf("isHLSDomainAllowed(%q) = %v, want %v", c.host, got, c.want)
			}
		})
	}
}
