package videoutils

import (
	"bytes"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSessTokenInjection asserts rewriteHLSURL appends a non-empty &sess=<token>
// param onto a rewritten segment URL, that the token is preserved verbatim
// across all segments of one manifest (one mint per manifest), that distinct
// manifest mints produce distinct tokens, and that an already-proxied URL is
// left untouched (existing skip rule preserved).
func TestSessTokenInjection(t *testing.T) {
	// A fresh manifest mint produces a non-empty crypto/rand token.
	tok1 := newSessToken()
	tok2 := newSessToken()
	assert.NotEmpty(t, tok1, "minted sess token must be non-empty")
	assert.NotEqual(t, tok1, tok2, "distinct manifest mints must produce distinct tokens")

	// rewriteHLSURL on a relative segment URL appends &sess=<token>.
	out := rewriteHLSURL("seg-1.ts", "https://cdn.example.com/path/", "https://ref.example/", tok1)
	assert.Contains(t, out, "sess="+tok1, "rewritten segment URL must carry &sess=<token>")
	assert.Contains(t, out, "/api/streaming/hls-proxy", "must still be a proxy URL")

	// All segments of the same manifest carry the SAME token.
	out2 := rewriteHLSURL("seg-2.ts", "https://cdn.example.com/path/", "https://ref.example/", tok1)
	assert.Contains(t, out2, "sess="+tok1, "second segment shares the manifest token")

	// An already-proxied URL is left untouched (skip rule preserved).
	already := "/api/streaming/hls-proxy?url=https%3A%2F%2Fcdn.example.com%2Fx.ts&sess=abc"
	assert.Equal(t, already, rewriteHLSURL(already, "https://cdn.example.com/path/", "", tok1),
		"already-proxied URL must be returned unchanged")

	// A whole-manifest rewrite mints exactly one token shared across its segments.
	manifest := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:6.0,",
		"a.ts",
		"#EXTINF:6.0,",
		"b.ts",
		"",
	}, "\n")
	rewritten := rewriteM3U8URLs(manifest, "https://cdn.example.com/path/playlist.m3u8", "https://ref.example/")
	// Extract every sess= value; they must all be identical and non-empty.
	var seen []string
	for _, line := range strings.Split(rewritten, "\n") {
		if i := strings.Index(line, "sess="); i != -1 {
			v := line[i+len("sess="):]
			if amp := strings.IndexByte(v, '&'); amp != -1 {
				v = v[:amp]
			}
			seen = append(seen, v)
		}
	}
	assert.Len(t, seen, 2, "two segment lines should each carry a sess token")
	if len(seen) == 2 {
		assert.NotEmpty(t, seen[0])
		assert.Equal(t, seen[0], seen[1], "both segments of one manifest share one minted token")
	}
}

// TestDualByteCount asserts the countReader wrapper increments a uint64 counter
// on each Read via atomic add (bytes_in source) — paired with the existing
// client-sink byte count (bytes_out), the proxy can report distinct, non-zero
// in/out totals where it reads upstream.
func TestDualByteCount(t *testing.T) {
	payload := []byte("the quick brown fox jumps over the lazy dog")
	var bytesIn uint64
	cr := &countReader{r: bytes.NewReader(payload), n: &bytesIn}

	var sink bytes.Buffer // stands in for the client ResponseWriter sink
	written, err := io.Copy(&sink, cr)
	assert.NoError(t, err)

	assert.Equal(t, int64(len(payload)), written, "client sink (bytes_out) wrote full payload")
	assert.Equal(t, uint64(len(payload)), bytesIn, "countReader (bytes_in) counted full upstream read")
	assert.Greater(t, bytesIn, uint64(0), "bytes_in must be non-zero")

	// Counter is safe under concurrent reads (atomic add, no data race).
	var racing uint64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := &countReader{r: bytes.NewReader(payload), n: &racing}
			_, _ = io.Copy(io.Discard, rc)
		}()
	}
	wg.Wait()
	assert.Equal(t, uint64(len(payload)*8), racing, "atomic counter sums all concurrent reads")
}

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

// TestRewriteVTTURLs_StoryboardCues asserts rewriteVTTURLs rewrites a
// storyboard thumbnail track's image cue payloads into signed proxy URLs
// while leaving the #xywh sprite-sheet fragment and timing lines untouched —
// the same treatment rewriteM3U8URLs already gives playlist children.
func TestRewriteVTTURLs_StoryboardCues(t *testing.T) {
	in := "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90\n\n00:00:05.000 --> 00:00:10.000\nstoryboard_001.jpg#xywh=160,0,160,90\n"
	out := rewriteVTTURLs(in, "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard.vtt", "", "")
	if !strings.Contains(out, "/api/streaming/hls-proxy?url="+url.QueryEscape("http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard_001.jpg")) {
		t.Fatalf("cue URL not proxied:\n%s", out)
	}
	if !strings.Contains(out, "#xywh=160,0,160,90") {
		t.Fatalf("xywh fragment must be preserved:\n%s", out)
	}
	if !strings.Contains(out, "&exp=") || !strings.Contains(out, "&sig=") {
		t.Fatalf("sheet URLs must carry provenance:\n%s", out)
	}
	if !strings.Contains(out, "00:00:00.000 --> 00:00:05.000") {
		t.Fatalf("timing lines must be untouched:\n%s", out)
	}
}

// TestRewriteVTTURLs_NonImagePayloadUntouched asserts a real subtitle VTT
// (non-storyboard, no image cue payloads) passes through rewriteVTTURLs
// byte-for-byte unchanged.
func TestRewriteVTTURLs_NonImagePayloadUntouched(t *testing.T) {
	in := "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\nSome subtitle text line\n"
	if out := rewriteVTTURLs(in, "http://x/s.vtt", "", ""); out != in {
		t.Fatalf("subtitle-style payload must pass through unchanged:\n%s", out)
	}
}
