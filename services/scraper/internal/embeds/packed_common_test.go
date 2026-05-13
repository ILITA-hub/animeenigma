// packed_common_test.go — base-type tests for the shared Dean-Edwards packer
// extractor. The packed_common.go base is consumed by StreamHGExtractor and
// EarnvidsExtractor; both differ only in name / allowlist / Referer.
//
// These tests are the GREEN counterpart to the Plan 18-01 scaffold step: the
// packedExtractor type lands in Plan 18-03 (this plan).
package embeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rewriteToSrv is a RoundTripper that preserves the request's allowlisted host
// (so Matches() and SSRF gates work against real hostnames like
// vibeplayer.site / otakuhg.site / otakuvid.online) while transparently routing
// the actual HTTP socket to a local httptest server. Without this scaffold,
// tests would have to either (a) bypass Matches() by injecting srv URLs that
// don't satisfy the host allowlist, OR (b) weaken Matches() to accept arbitrary
// hosts. Both options defeat the SSRF contract — rewriteToSrv is the ONLY
// pattern that keeps Matches() strict while serving offline fixtures.
type rewriteToSrv struct{ srvURL string }

func (r *rewriteToSrv) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(r.srvURL)
	if err != nil {
		return nil, err
	}
	// Mutate only the transport-relevant fields — preserve Path, Query, Scheme
	// case, etc. so the upstream handler sees a request shape identical to what
	// it would receive in production.
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}

// TestPackedExtractor_Matches_RejectsSubdomainImposters verifies the SSRF gate
// at the packedExtractor base level: substring-match impostors must NOT match.
func TestPackedExtractor_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Parallel()
	e := &packedExtractor{hosts: []string{"example.test"}}
	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.test/x", true},        // exact host
		{"https://sub.example.test/x", true},    // strict subdomain
		{"https://EXAMPLE.test/x", true},        // case-insensitive
		{"https://evilexample.test/x", false},   // impostor — no leading dot
		{"https://example.test.evil.com", false},// suffix attack
		{"ftp://example.test/x", false},         // wrong scheme
		{"https:///x", false},                   // empty host
		{"", false},                             // empty URL
		{"://no-scheme", false},                 // malformed
	}
	for _, c := range cases {
		c := c
		t.Run(c.url, func(t *testing.T) {
			t.Parallel()
			if got := e.Matches(c.url); got != c.want {
				t.Errorf("Matches(%q) = %v; want %v", c.url, got, c.want)
			}
		})
	}
}

// TestExtractAllPlayableURLs_MultiURL — Plan 22-01 Task 1 RED.
// Asserts the helper extracts BOTH hls2 (signed m3u8) and hls3 (unsigned txt)
// URLs from a synthetic unpacked dictionary, ordered hls2-first.
func TestExtractAllPlayableURLs_MultiURL(t *testing.T) {
	t.Parallel()
	unpacked := `{"hls2":"https://x.premilkyway.com/a/master.m3u8?e=123","hls3":"https://managementadvisory.sbs/u/hls3/m/master.txt"}`
	got := extractAllPlayableURLs(unpacked)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2; got=%#v", len(got), got)
	}
	if !strings.HasSuffix(strings.Split(got[0].URL, "?")[0], ".m3u8") {
		t.Errorf("got[0].URL = %q; want suffix .m3u8 (before query)", got[0].URL)
	}
	if !strings.HasSuffix(got[1].URL, ".txt") {
		t.Errorf("got[1].URL = %q; want suffix .txt", got[1].URL)
	}
	for i, s := range got {
		if s.Type != "hls" {
			t.Errorf("got[%d].Type = %q; want hls", i, s.Type)
		}
	}
}

// TestExtractAllPlayableURLs_Hls2Only — only hls2 present → length 1.
func TestExtractAllPlayableURLs_Hls2Only(t *testing.T) {
	t.Parallel()
	unpacked := `{"hls2":"https://x.premilkyway.com/a/master.m3u8?e=123"}`
	got := extractAllPlayableURLs(unpacked)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d; want 1; got=%#v", len(got), got)
	}
	if !strings.HasSuffix(strings.Split(got[0].URL, "?")[0], ".m3u8") {
		t.Errorf("got[0].URL = %q; want suffix .m3u8 (before query)", got[0].URL)
	}
}

// TestExtractAllPlayableURLs_Empty — no URLs → nil.
func TestExtractAllPlayableURLs_Empty(t *testing.T) {
	t.Parallel()
	if got := extractAllPlayableURLs(`{"foo":"bar"}`); got != nil {
		t.Errorf("got = %#v; want nil", got)
	}
}

// TestExtractAllPlayableURLs_Dedupe — duplicate URLs collapse to one entry.
func TestExtractAllPlayableURLs_Dedupe(t *testing.T) {
	t.Parallel()
	unpacked := `{"hls2":"https://x.premilkyway.com/master.m3u8?e=1","hls3":"https://x.premilkyway.com/master.m3u8?e=1"}`
	got := extractAllPlayableURLs(unpacked)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d; want 1 (dedup); got=%#v", len(got), got)
	}
}

// TestExtractAllPlayableURLs_KeyRotation — packed JS rotates the literal
// "hls2"/"hls3" keys to "streamA"/"streamB"; the fallback regex still
// extracts the playable URL.
func TestExtractAllPlayableURLs_KeyRotation(t *testing.T) {
	t.Parallel()
	unpacked := `{"streamA":"https://x.premilkyway.com/master.m3u8?e=1"}`
	got := extractAllPlayableURLs(unpacked)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d; want 1 (fallback regex); got=%#v", len(got), got)
	}
	if !strings.HasSuffix(strings.Split(got[0].URL, "?")[0], ".m3u8") {
		t.Errorf("got[0].URL = %q; want suffix .m3u8 (before query)", got[0].URL)
	}
}

// TestExtractAllPlayableURLs_OrderingHls2First — when both keys present,
// hls2 always precedes hls3 regardless of textual order in the input.
func TestExtractAllPlayableURLs_OrderingHls2First(t *testing.T) {
	t.Parallel()
	unpacked := `{"hls3":"https://h3.example/master.txt","hls2":"https://h2.example/master.m3u8?e=1"}`
	got := extractAllPlayableURLs(unpacked)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2; got=%#v", len(got), got)
	}
	if !strings.Contains(got[0].URL, ".m3u8") {
		t.Errorf("got[0].URL = %q; want hls2 (m3u8) first regardless of source order", got[0].URL)
	}
	if !strings.HasSuffix(got[1].URL, ".txt") {
		t.Errorf("got[1].URL = %q; want hls3 (txt) second", got[1].URL)
	}
}

// TestExtractAllPlayableURLs_FallbackCapped — hostile body stuffs 50 URLs;
// the fallback regex pass is capped at 5 entries (T-22-02 DoS guard).
func TestExtractAllPlayableURLs_FallbackCapped(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 50; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"rotKey`)
		b.WriteString(fmt.Sprintf("%d", i))
		b.WriteString(`":"https://h.example/x`)
		b.WriteString(fmt.Sprintf("%d", i))
		b.WriteString(`.m3u8"`)
	}
	b.WriteString("}")
	got := extractAllPlayableURLs(b.String())
	if len(got) > 5 {
		t.Errorf("len(got) = %d; want ≤ 5 (T-22-02 DoS guard cap)", len(got))
	}
}

// TestPackedExtractor_Extract_FromGolden serves the captured streamhg packed
// HTML fixture from an httptest server, points the extractor at the
// allowlisted host (so Matches() succeeds), and routes the actual HTTP socket
// via the rewriteToSrv RoundTripper.
func TestPackedExtractor_Extract_FromGolden(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "testdata", "gogoanime", "streamhg_packed.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	// We invoke Extract against the real allowlisted host name so Matches()
	// succeeds; the rewriteToSrv RoundTripper redirects the actual TCP socket
	// to the local httptest server.
	e := &packedExtractor{
		name:               "packed-test",
		hosts:              []string{"otakuhg.site"},
		referer:            "https://otakuhg.site/",
		selectorPackerFail: "packer_balance",
		selectorRegexFail:  "hls2_regex",
		selectorBodyFail:   "body_read",
		http: &http.Client{
			Transport: &rewriteToSrv{srvURL: srv.URL},
			Timeout:   5 * time.Second,
		},
		timeout: 5 * time.Second,
	}
	stream, err := e.Extract(context.Background(), "https://otakuhg.site/embed-abc.html", http.Header{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatalf("Extract returned empty stream: %+v", stream)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("source type = %q, want hls", stream.Sources[0].Type)
	}
	if !strings.Contains(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("source URL = %q, want substring .m3u8", stream.Sources[0].URL)
	}
	if stream.Headers["Referer"] != "https://otakuhg.site/" {
		t.Errorf("Referer header = %q, want https://otakuhg.site/", stream.Headers["Referer"])
	}
}
