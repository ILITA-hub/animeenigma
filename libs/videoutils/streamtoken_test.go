package videoutils

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreamToken_RoundTrip(t *testing.T) {
	now := time.Now()
	tok := EncodeStreamToken("https://p12.solodcdn.com/s/m/seg-1.ts", "https://kodikplayer.com/", "", now)
	if tok == "" {
		t.Fatal("expected non-empty token with configured secret")
	}
	if strings.ContainsAny(tok, "/+=") {
		t.Fatalf("token must be a single URL-safe path segment, got %q", tok)
	}
	p, err := DecodeStreamToken(tok, now)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.URL != "https://p12.solodcdn.com/s/m/seg-1.ts" || p.Referer != "https://kodikplayer.com/" || p.Type != "" {
		t.Fatalf("payload mismatch: %+v", p)
	}
}

func TestStreamToken_CarriesType(t *testing.T) {
	tok := EncodeStreamToken("https://video.sibnet.ru/v/ep1.mp4", "https://video.sibnet.ru/", "mp4", time.Now())
	p, err := DecodeStreamToken(tok, time.Now())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Type != "mp4" {
		t.Fatalf("Type = %q; want mp4", p.Type)
	}
}

func TestStreamToken_RejectsTamper(t *testing.T) {
	tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", time.Now())
	// Flip a character in the middle of the token.
	b := []byte(tok)
	mid := len(b) / 2
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	if _, err := DecodeStreamToken(string(b), time.Now()); err == nil {
		t.Fatal("tampered token must not decode")
	}
	if _, err := DecodeStreamToken("garbage!!!", time.Now()); err == nil {
		t.Fatal("garbage must not decode")
	}
}

func TestStreamToken_RejectsExpired(t *testing.T) {
	past := time.Now().Add(-2 * provenanceTTL)
	tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", past)
	if _, err := DecodeStreamToken(tok, time.Now()); err == nil {
		t.Fatal("expired token must not decode")
	}
}

func TestStreamToken_FailsClosedWhenUnconfigured(t *testing.T) {
	savedSecret, savedConfigured := provenanceSecret, provenanceConfigured
	defer func() {
		provenanceSecret, provenanceConfigured = savedSecret, savedConfigured
		streamTokenAEAD = nil
		streamTokenAEADOnce = sync.Once{}
	}()
	provenanceSecret, provenanceConfigured = nil, false
	streamTokenAEAD = nil
	streamTokenAEADOnce = sync.Once{}
	if tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", time.Now()); tok != "" {
		t.Fatalf("expected empty token when unconfigured, got %q", tok)
	}
	if _, err := DecodeStreamToken("anything", time.Now()); err == nil {
		t.Fatal("expected decode error when unconfigured")
	}
}

func TestMaskedStreamURL_ShapeAndLeaf(t *testing.T) {
	u := MaskedStreamURL("https://p12.solodcdn.com/s/m/720.mp4:hls:seg-225-v1-a1.ts", "https://kodikplayer.com/", "")
	if !strings.HasPrefix(u, "/api/streaming/m/") {
		t.Fatalf("masked URL prefix wrong: %q", u)
	}
	if strings.Contains(u, "url=") || strings.Contains(u, "solodcdn") {
		t.Fatalf("masked URL leaks upstream shape: %q", u)
	}
	if !strings.HasSuffix(u, ".ts") {
		t.Fatalf("leaf extension lost (player heuristics need it): %q", u)
	}
}

func TestMaskedLeaf(t *testing.T) {
	cases := map[string]string{
		"https://cdn.example.com/a/b/manifest.m3u8": "manifest.m3u8",
		"https://cdn.example.com/":                  "media",
		"://bad":                                    "media",
	}
	for in, want := range cases {
		if got := maskedLeaf(in); got != want {
			t.Errorf("maskedLeaf(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestRewriteHLSURL_EmitsMaskedForm(t *testing.T) {
	out := rewriteHLSURL("seg-1.ts", "https://cdn.example.com/ep1/", "https://kodikplayer.com/", "abc123")
	if !strings.HasPrefix(out, "/api/streaming/m/") {
		t.Fatalf("expected masked form, got %q", out)
	}
	if strings.Contains(out, "url=") || strings.Contains(out, "cdn.example.com") || strings.Contains(out, "hls-proxy") {
		t.Fatalf("masked child URL leaks legacy shape: %q", out)
	}
	if !strings.HasSuffix(out, "?sess=abc123") {
		t.Fatalf("sess correlation param missing: %q", out)
	}
	// The token must round-trip to the absolute segment URL + referer.
	tok := strings.TrimPrefix(out, "/api/streaming/m/")
	tok = tok[:strings.Index(tok, "/")]
	p, err := DecodeStreamToken(tok, time.Now())
	if err != nil {
		t.Fatalf("emitted token does not decode: %v", err)
	}
	if p.URL != "https://cdn.example.com/ep1/seg-1.ts" {
		t.Fatalf("token URL = %q", p.URL)
	}
	if p.Referer != "https://kodikplayer.com/" {
		t.Fatalf("token referer = %q", p.Referer)
	}
	// Leaf keeps the extension for player heuristics.
	if !strings.Contains(out, "/seg-1.ts?") {
		t.Fatalf("leaf lost: %q", out)
	}
}

func TestRewriteHLSURL_SkipsAlreadyMasked(t *testing.T) {
	in := "/api/streaming/m/sometoken/seg-1.ts?sess=x"
	if out := rewriteHLSURL(in, "https://cdn.example.com/", "", "y"); out != in {
		t.Fatalf("already-masked URL must pass through, got %q", out)
	}
}

func TestRewriteHLSURL_LegacyFallbackWhenDisabled(t *testing.T) {
	savedSecret, savedConfigured := provenanceSecret, provenanceConfigured
	defer func() {
		provenanceSecret, provenanceConfigured = savedSecret, savedConfigured
		streamTokenAEAD = nil
		streamTokenAEADOnce = sync.Once{}
	}()
	provenanceSecret, provenanceConfigured = nil, false
	streamTokenAEAD = nil
	streamTokenAEADOnce = sync.Once{}

	out := rewriteHLSURL("seg-1.ts", "https://cdn.example.com/ep1/", "", "abc")
	if !strings.HasPrefix(out, "/api/streaming/hls-proxy?url=") {
		t.Fatalf("expected legacy fallback when tokens disabled, got %q", out)
	}
}
