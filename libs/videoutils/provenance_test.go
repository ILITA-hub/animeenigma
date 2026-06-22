package videoutils

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProvenanceToken_RoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	raw := "https://oh7s.lookaround.click/anime/abc/seg-1.ts"
	exp, sig := signProvenance(raw, now)
	if !validProvenanceToken(raw, exp, sig, now) {
		t.Fatal("fresh token should validate")
	}
	// Just before expiry → valid; just after → invalid.
	if !validProvenanceToken(raw, exp, sig, now.Add(provenanceTTL-time.Second)) {
		t.Error("token should be valid just before expiry")
	}
	if validProvenanceToken(raw, exp, sig, now.Add(provenanceTTL+time.Second)) {
		t.Error("token should be invalid after expiry")
	}
}

func TestProvenanceToken_RejectsTamperAndGarbage(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	raw := "https://oh7s.lookaround.click/anime/abc/seg-1.ts"
	exp, sig := signProvenance(raw, now)

	cases := map[string]bool{
		"different url":  validProvenanceToken("https://evil.example/seg-1.ts", exp, sig, now),
		"tampered sig":   validProvenanceToken(raw, exp, sig[:len(sig)-1]+"0", now),
		"empty sig":      validProvenanceToken(raw, exp, "", now),
		"empty exp":      validProvenanceToken(raw, "", sig, now),
		"nonnumeric exp": validProvenanceToken(raw, "notanumber", sig, now),
	}
	for name, got := range cases {
		if got {
			t.Errorf("%s: token unexpectedly validated", name)
		}
	}
}

// TestProvenance_FailsClosedWhenUnconfigured proves that with no signing
// secret configured the token mechanism is DISABLED: minting yields an empty
// token and validation rejects everything — including a token forged from the
// old public hardcoded default. This is the fix for the SSRF/open-proxy hole
// where a missing secret silently fell back to a guessable default.
func TestProvenance_FailsClosedWhenUnconfigured(t *testing.T) {
	_ = provenanceEnabled() // ensure the sync.Once has fired (TestMain set a secret)
	origSecret, origConfigured := provenanceSecret, provenanceConfigured
	t.Cleanup(func() { provenanceSecret, provenanceConfigured = origSecret, origConfigured })

	// Simulate "no secret configured". The Once has already fired, so
	// loadProvenanceSecret is a no-op and these values stick for the test.
	provenanceSecret, provenanceConfigured = nil, false

	if provenanceEnabled() {
		t.Fatal("provenance must be disabled when no secret is configured")
	}
	now := time.Unix(1_700_000_000, 0)
	raw := "https://attacker.example/internal/seg.ts"
	if exp, sig := signProvenance(raw, now); exp != "" || sig != "" {
		t.Fatalf("signProvenance must mint an empty token when disabled; got exp=%q sig=%q", exp, sig)
	}
	// A token forged with the OLD public default secret must NOT validate.
	fm := hmac.New(sha256.New, []byte("animeenigma-hls-provenance-default"))
	expStr := strconv.FormatInt(now.Add(time.Hour).Unix(), 10)
	fm.Write([]byte(raw))
	fm.Write([]byte("\n"))
	fm.Write([]byte(expStr))
	forgedSig := hex.EncodeToString(fm.Sum(nil))[:32]
	if validProvenanceToken(raw, expStr, forgedSig, now) {
		t.Fatal("a token forged from the old hardcoded default must be rejected when disabled")
	}
}

// TestRewriteM3U8URLs_MintsValidToken verifies that rewriting a playlist
// stamps every rewritten URL with an &exp=&sig= that authenticates the
// absolute target URL — the property the segment-host bypass relies on.
func TestRewriteM3U8URLs_MintsValidToken(t *testing.T) {
	master := "https://cdn.mewstream.buzz/anime/abc/master.m3u8"
	playlist := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nhttps://oh7s.lookaround.click/anime/abc/index.m3u8\n"
	out := rewriteM3U8URLs(playlist, master, "https://megaplay.buzz/")

	// Find the rewritten proxy line and pull its url/exp/sig.
	var proxyLine string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "hls-proxy") {
			proxyLine = strings.TrimSpace(l)
		}
	}
	if proxyLine == "" {
		t.Fatalf("no rewritten proxy line in:\n%s", out)
	}
	q, err := url.Parse(proxyLine)
	if err != nil {
		t.Fatalf("parse proxy line: %v", err)
	}
	vals := q.Query()
	target := vals.Get("url")
	if target != "https://oh7s.lookaround.click/anime/abc/index.m3u8" {
		t.Fatalf("rewritten url = %q; want the absolute segment URL", target)
	}
	if !validProvenanceToken(target, vals.Get("exp"), vals.Get("sig"), time.Now()) {
		t.Errorf("minted token does not validate for %q (exp=%q sig=%q)", target, vals.Get("exp"), vals.Get("sig"))
	}
}

// TestProxyWithReferer_TokenBypassesAllowlist proves a non-allowlisted host
// is served WHEN it carries a valid provenance token, and rejected without.
func TestProxyWithReferer_TokenBypassesAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("SEGMENTDATA"))
	}))
	defer upstream.Close()

	// upstream.URL host (127.0.0.1:port) is NOT in the allowlist.
	proxy := NewVideoProxy(DefaultProxyConfig())
	segURL := upstream.URL + "/anime/abc/seg-1.ts"

	// Without a token → DomainNotAllowedError.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-proxy?url="+url.QueryEscape(segURL), nil)
		err := proxy.ProxyWithReferer(context.Background(), segURL, "https://megaplay.buzz/", rec, req)
		if _, ok := err.(*DomainNotAllowedError); !ok {
			t.Fatalf("no-token request: err = %v; want *DomainNotAllowedError", err)
		}
	}

	// With a valid token → served (200, body proxied).
	{
		exp, sig := signProvenance(segURL, time.Now())
		rec := httptest.NewRecorder()
		reqURL := "/api/v1/hls-proxy?url=" + url.QueryEscape(segURL) + "&exp=" + exp + "&sig=" + sig
		req := httptest.NewRequest(http.MethodGet, reqURL, nil)
		if err := proxy.ProxyWithReferer(context.Background(), segURL, "https://megaplay.buzz/", rec, req); err != nil {
			t.Fatalf("token request: unexpected err %v", err)
		}
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SEGMENTDATA") {
			t.Fatalf("token request: code=%d body=%q; want 200 + segment body", rec.Code, rec.Body.String())
		}
	}
}

// TestSignStreamURL_SurvivesQueryRoundTrip locks the critical invariant: a URL
// signed by the catalog (SignStreamURL) still validates after the frontend
// places it in the `url` query param and the proxy reads it back via
// url.Values. Uses reserved chars (% & = + space) that exercise the encoder.
func TestSignStreamURL_SurvivesQueryRoundTrip(t *testing.T) {
	raw := "https://cdn.example.com/path with space/master.m3u8?token=a+b%2Fc&exp=1&q=x=y"
	exp, sig := SignStreamURL(raw)
	if exp == "" || sig == "" {
		t.Fatal("SignStreamURL returned empty exp/sig")
	}

	// Frontend side: place raw into the `url` param with standard encoding.
	out := url.Values{}
	out.Set("url", raw)
	out.Set("exp", exp)
	out.Set("sig", sig)
	encoded := out.Encode()

	// Proxy side: parse the query back and validate over the decoded url.
	parsed, err := url.ParseQuery(encoded)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	gotURL := parsed.Get("url")
	if gotURL != raw {
		t.Fatalf("url round-trip changed the string:\n got:  %q\n want: %q", gotURL, raw)
	}
	if !validProvenanceToken(gotURL, parsed.Get("exp"), parsed.Get("sig"), time.Now()) {
		t.Fatal("token rejected after query round-trip (encoding-invariant broken)")
	}
	// Tampered sig must fail.
	if validProvenanceToken(gotURL, parsed.Get("exp"), "deadbeef"+parsed.Get("sig")[8:], time.Now()) {
		t.Fatal("tampered sig validated; want reject")
	}
}
