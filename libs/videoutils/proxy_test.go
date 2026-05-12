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
