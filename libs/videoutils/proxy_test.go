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

// TestMatchDomainPattern keeps SSRF-contract coverage for the pattern matcher
// that still gates the legacy signed-token path (ProxyConfig.AllowedDomains /
// PROXY_ALLOWED_DOMAINS): the leading dot in the HasSuffix rule rejects prefix
// impostors (evilexample.com), eTLD+1 patterns never match a host that merely
// EMBEDS the allowed domain (example.com.attacker.com), and prefix wildcards
// stay anchored to their suffix TLD (cdn-evil.attacker.io is rejected).
// The HLS trust gate itself no longer consults any static list — see
// TestTrustGate_* — this matcher survives only for the legacy path.
func TestMatchDomainPattern(t *testing.T) {
	cases := []struct {
		host    string
		pattern string
		want    bool
		label   string
	}{
		{"example.com", "example.com", true, "exact match"},
		{"cdn.example.com", "example.com", true, "subdomain match"},
		{"a.b.example.com", "example.com", true, "deep subdomain match"},
		{"evilexample.com", "example.com", false, "prefix impostor rejected"},
		{"example.com.attacker.com", "example.com", false, "embedded-domain impostor rejected"},
		{"example.org", "example.com", false, "different TLD rejected"},
		{"sub.example.com", "*.example.com", true, "star-dot wildcard subdomain"},
		{"cdn-belias.com", "cdn-*.com", true, "anchored prefix wildcard — leftmost label"},
		{"edge.cdn-hydaelyn.com", "cdn-*.com", true, "anchored prefix wildcard — inner label"},
		{"cdn-evil.attacker.io", "cdn-*.com", false, "wildcard prefix on wrong TLD rejected"},
		{"notcdn-belias.com", "cdn-*.com", false, "prefix must begin a DNS label"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.label, func(t *testing.T) {
			if got := matchDomainPattern(c.host, c.pattern); got != c.want {
				t.Errorf("matchDomainPattern(%q, %q) = %v, want %v", c.host, c.pattern, got, c.want)
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
