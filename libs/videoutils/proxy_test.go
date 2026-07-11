package videoutils

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

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
	// "must still be a proxy URL" now means the opaque masked path form
	// (Track A) — decode its token and confirm it resolves to the absolute
	// upstream segment URL, same proxied+signed invariant as the legacy
	// hls-proxy?url=&exp=&sig= shape it replaces.
	if assert.True(t, strings.HasPrefix(out, "/api/streaming/m/"), "must still be a proxy URL") {
		tok := strings.TrimPrefix(out, "/api/streaming/m/")
		tok = tok[:strings.Index(tok, "/")]
		p, err := DecodeStreamToken(tok, time.Now())
		if assert.NoError(t, err, "rewritten segment URL's token must decode") {
			assert.Equal(t, "https://cdn.example.com/path/seg-1.ts", p.URL, "token must resolve to the absolute upstream URL")
		}
	}

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
		"hanime.tv",
		"jimaku.cc",
		"cdnlibs.org",
		"solodcdn.com",
		"mp4upload.com",
		"turboviplay.com",
	}

	for _, domain := range knownDomains {
		assert.True(t, isHLSDomainAllowed(domain), "known domain %q should be allowed", domain)
	}
}

func TestIsHLSDomainAllowed_KnownSubdomains(t *testing.T) {
	assert.True(t, isHLSDomainAllowed("cdn.hanime.tv"))
	assert.True(t, isHLSDomainAllowed("a1.mp4upload.com"))
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
	assert.True(t, isHLSDomainAllowed("hanime.tv:443"), "should strip port and match domain")
	assert.True(t, isHLSDomainAllowed("jimaku.cc:8080"), "should strip port and match domain")
	assert.True(t, isHLSDomainAllowed("minio:9000"), "should strip port and match first-party host")
	assert.False(t, isHLSDomainAllowed("evil.com:443"), "should strip port but still reject unknown domain")
}

// TestHLSProxyAllowedDomains_UnsignedPathHostsLocked is the regression lock
// for the post-phase-out allow-list. Every entry here backs a path that
// CANNOT ride signed-URL provenance yet: first-party docker-network hosts
// plus catalog endpoints that return URLs unsigned (Kodik ad-free, Hanime,
// AnimeLib, 18anime, subtitles). Removing one of these 403s that source;
// a signed scraper CDN showing up here means the phase-out regressed —
// scraper CDNs must NOT be re-added (they ride streamsign provenance).
func TestHLSProxyAllowedDomains_UnsignedPathHostsLocked(t *testing.T) {
	want := []string{
		// first-party
		"stealth-scraper", "minio",
		// Kodik ad-free HLS (unsigned catalog path)
		"solodcdn.com", "cloud.solodcdn.com",
		// Hanime CDN family (unsigned catalog path)
		"hanime.tv", "highwinds-cdn.com", "htv-*.com", "hydaelyn-*.top", "zodiark-*.top",
		// AnimeLib CDNs (unsigned catalog path)
		"cdnlibs.org", "hentaicdn.org",
		// 18anime embed mirrors (Get18AnimeStream strips provenance)
		"mp4upload.com", "turboviplay.com", "turbosplayer.com",
		// Japanese subtitle files (unsigned subtitle endpoints)
		"jimaku.cc",
		// AUTO-517 stop-gap (redirect target re-gated without a token)
		"mt.nekostream.site",
	}
	present := make(map[string]bool, len(HLSProxyAllowedDomains))
	for _, d := range HLSProxyAllowedDomains {
		present[d] = true
	}
	for _, d := range want {
		if !present[d] {
			t.Errorf("HLSProxyAllowedDomains missing unsigned-path host %q — that source would 403", d)
		}
	}
	// Set-equality tripwire: any entry not in want fails BY NAME. Adding a
	// host for a new unsigned serving path is legitimate — extend want in
	// the same commit (deliberate). A scraper CDN must never be re-added;
	// those ride streamsign provenance.
	expected := make(map[string]bool, len(want))
	for _, d := range want {
		expected[d] = true
	}
	for _, d := range HLSProxyAllowedDomains {
		if !expected[d] {
			t.Errorf("HLSProxyAllowedDomains has unexpected entry %q — scraper CDNs ride provenance "+
				"signing; a new unsigned catalog path must extend this test's want list deliberately", d)
		}
	}
}

// TestIsHLSDomainAllowed_ImpostorRejection pins the SSRF contract of the
// gate: the leading dot in the HasSuffix rule rejects prefix impostors
// (evilhanime.tv), and eTLD+1 entries never match a host that merely
// EMBEDS the allowed domain (hanime.tv.attacker.com).
func TestIsHLSDomainAllowed_ImpostorRejection(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"hanime.tv", true},
		{"cdn.hanime.tv", true},
		{"a.b.hanime.tv", true},
		{"cdn.hanime.tv:443", true},
		{"hanime.com", false},
		{"evilhanime.tv", false},
		{"hanime.tv.attacker.com", false},
		{"solodcdn.com", true},
		{"draco.cloud.solodcdn.com", true},
		{"solodcdn.org", false},
		{"evilsolodcdn.com", false},
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

// TestIsHLSDomainAllowed_AnchoredPrefixWildcards exercises the anchored
// prefix-wildcard patterns carried by the Hanime CDN family (htv-*.com,
// hydaelyn-*.top, zodiark-*.top). The suffix anchor is the SSRF defense:
// a matching prefix on the wrong TLD (htv-evil.attacker.io) must be
// rejected — see matchHLSDomain.
func TestIsHLSDomainAllowed_AnchoredPrefixWildcards(t *testing.T) {
	cases := []struct {
		host  string
		want  bool
		label string
	}{
		{"htv-belias.com", true, "htv wildcard — leftmost label"},
		{"edge.htv-hydaelyn.com", true, "htv wildcard — inner label"},
		{"hydaelyn-25x-07.top", true, "hydaelyn wildcard exact"},
		{"zodiark-25x-03.top", true, "zodiark wildcard exact"},
		{"htv-evil.attacker.io", false, "wildcard prefix on wrong TLD"},
		{"nothtv-belias.com", false, "prefix must begin a DNS label"},
		{"hydaelyn-25x-07.com", false, "hydaelyn wildcard anchored to .top"},
		{"jimaku.cc", true, "subtitle host exact"},
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
	out := rewriteVTTURLs(in, "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard.vtt", "")
	if !strings.Contains(out, "/api/streaming/m/") {
		t.Fatalf("cue URL not proxied:\n%s", out)
	}
	if !strings.Contains(out, "#xywh=160,0,160,90") {
		t.Fatalf("xywh fragment must be preserved:\n%s", out)
	}
	// Provenance now rides sealed inside the masked token's AEAD payload
	// (expiry is part of the sealed StreamTokenPayload) rather than as
	// separate &exp=/&sig= query params — decode the token from the first
	// rewritten cue line and confirm it resolves to the expected absolute
	// upstream sheet URL (same "proxied and signed" invariant, masked shape).
	var cueLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "/api/streaming/m/") {
			cueLine = line
			break
		}
	}
	if cueLine == "" {
		t.Fatalf("no rewritten cue line in:\n%s", out)
	}
	tok := strings.TrimPrefix(cueLine, "/api/streaming/m/")
	tok = tok[:strings.Index(tok, "/")]
	p, err := DecodeStreamToken(tok, time.Now())
	if err != nil {
		t.Fatalf("sheet URL token does not decode: %v", err)
	}
	if p.URL != "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard_001.jpg" {
		t.Fatalf("token URL = %q", p.URL)
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
	if out := rewriteVTTURLs(in, "http://x/s.vtt", ""); out != in {
		t.Fatalf("subtitle-style payload must pass through unchanged:\n%s", out)
	}
}

// TestGetCorrectHLSContentType_FirstPartyImagesStayImages pins the first-party
// image exemption: genuine images from trusted hosts (MinIO sprite sheets,
// admin uploads) must NOT be relabeled video/mp2t by the image→video
// obfuscation heuristic (image bytes served as video break under any future
// nosniff header), while the heuristic keeps firing for third-party CDN
// segments that merely claim to be images.
func TestGetCorrectHLSContentType_FirstPartyImagesStayImages(t *testing.T) {
	// First-party sprite sheet keeps its declared image type.
	if got := getCorrectHLSContentType("/raw-library/aeProvider/1/RAW/1/storyboard_001.jpg", "image/jpeg", true); got != "image/jpeg" {
		t.Fatalf("first-party sheet content-type = %q, want image/jpeg", got)
	}
	// Generalizes beyond the storyboard naming — any first-party image asset.
	if got := getCorrectHLSContentType("/raw-library/posters/cover.png", "image/png", true); got != "image/png" {
		t.Fatalf("first-party png content-type = %q, want image/png", got)
	}
	// Third-party CDN pretending to be an image keeps the video override.
	if got := getCorrectHLSContentType("/cdn/seg-42.bin", "image/jpeg", false); got != "video/mp2t" {
		t.Fatalf("obfuscated segment content-type = %q, want video/mp2t", got)
	}
	// Even an image-extension path with an image type is NOT trusted third-party.
	if got := getCorrectHLSContentType("/cdn/fake_frame.jpg", "image/jpeg", false); got != "video/mp2t" {
		t.Fatalf("untrusted jpg content-type = %q, want video/mp2t", got)
	}
	// First-party VIDEO content is still normalized — the exemption is image-scoped.
	if got := getCorrectHLSContentType("/raw-library/aeProvider/1/RAW/1/segment_000.ts", "application/octet-stream", true); got != "video/mp2t" {
		t.Fatalf("first-party segment content-type = %q, want video/mp2t", got)
	}
}
