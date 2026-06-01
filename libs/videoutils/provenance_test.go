package videoutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
