// earnvids_test.go — GREEN tests for EarnvidsExtractor (Dean-Edwards packer).
//
// SCRAPER-9ANI-03 (SSRF gate) + SCRAPER-9ANI-04 (Extract from offline golden).
// Mirrors streamhg_test.go modulo allowlist (otakuvid.online) and CDN host
// (dramiyos-cdn.com vs premilkyway.com). Reuses the rewriteToSrv RoundTripper
// defined in packed_common_test.go.
package embeds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// earnvidsGolden resolves the earnvids_packed.html golden captured in Plan
// 18-01 Task 3 (path: services/scraper/testdata/gogoanime/earnvids_packed.html).
func earnvidsGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "earnvids_packed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestEarnvids_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate at the Earnvids-specific allowlist.
func TestEarnvids_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Parallel()
	e := NewEarnvidsExtractor()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://otakuvid.online/abc", true},
		{"https://cdn.otakuvid.online/abc", true},
		{"HTTPS://OTAKUVID.ONLINE/abc", true},
		{"https://evilotakuvid.online/abc", false},
		{"https://otakuvid.com/abc", false},
		{"https://otakuvid.online.evil.com", false},
		{"ftp://otakuvid.online/abc", false},
		{"https:///abc", false},
		{"", false},
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

// TestEarnvids_Name pins the stable identifier emitted in logs / metrics.
func TestEarnvids_Name(t *testing.T) {
	t.Parallel()
	if got := NewEarnvidsExtractor().Name(); got != "earnvids" {
		t.Errorf("Name() = %q; want %q", got, "earnvids")
	}
}

// TestEarnvids_Extract_FromGolden verifies SCRAPER-9ANI-04: Earnvids
// shares the Dean-Edwards packer unpack flow with StreamHG; differs only by
// host allowlist (otakuvid.online) and CDN (dramiyos-cdn.com instead of
// premilkyway.com).
func TestEarnvids_Extract_FromGolden(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(earnvidsGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewEarnvidsExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://otakuvid.online/d/tqcjvlkmh41z",
		http.Header{},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatalf("Extract returned empty stream: %+v", stream)
	}
	if !strings.HasSuffix(strings.Split(stream.Sources[0].URL, "?")[0], ".m3u8") {
		t.Errorf("source URL path = %q; want suffix .m3u8 (before query)", stream.Sources[0].URL)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("source type = %q; want hls", stream.Sources[0].Type)
	}
	if stream.Headers["Referer"] != "https://otakuvid.online/" {
		t.Errorf("Referer header = %q; want https://otakuvid.online/", stream.Headers["Referer"])
	}
	// Earnvids URL carries `e=` expiry param (same shape as StreamHG).
	if !strings.Contains(stream.Sources[0].URL, "e=") {
		t.Errorf("source URL = %q; want substring 'e=' (signed-expiry param)", stream.Sources[0].URL)
	}
}

// TestEarnvids_MultiURL_FromGolden — Plan 22-01 Task 1. Mirror of the
// streamhg multi-URL test against the earnvids golden. hls3 host here is
// enterpriseconsulting.sbs (different from the spec's
// exoplanethunting.space — the committed golden was captured 2026-05-12
// and uses a different rotation of the hls3 CDN).
func TestEarnvids_MultiURL_FromGolden(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(earnvidsGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewEarnvidsExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://otakuvid.online/d/tqcjvlkmh41z",
		http.Header{},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) < 2 {
		t.Fatalf("len(Sources) < 2; got %d sources; full=%+v", len(stream.Sources), stream)
	}
	if !strings.Contains(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("Sources[0].URL = %q; want substring .m3u8 (hls2 first)", stream.Sources[0].URL)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("Sources[0].Type = %q; want hls", stream.Sources[0].Type)
	}
	foundHls3 := false
	for _, s := range stream.Sources {
		if strings.Contains(s.URL, "enterpriseconsulting.sbs") && strings.HasSuffix(s.URL, ".txt") {
			foundHls3 = true
			if s.Type != "hls" {
				t.Errorf("hls3 Source Type = %q; want hls", s.Type)
			}
			break
		}
	}
	if !foundHls3 {
		t.Errorf("no Source contains enterpriseconsulting.sbs *.txt URL; sources=%+v", stream.Sources)
	}
	if stream.Headers["Referer"] != "https://otakuvid.online/" {
		t.Errorf("Referer header = %q; want https://otakuvid.online/", stream.Headers["Referer"])
	}
}

// TestEarnvids_MultiURL_Order — hls2 (.m3u8) precedes hls3 (.txt) in Sources.
func TestEarnvids_MultiURL_Order(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(earnvidsGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewEarnvidsExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(context.Background(), "https://otakuvid.online/d/tqcjvlkmh41z", http.Header{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(stream.Sources) < 2 {
		t.Fatalf("len(Sources) = %d; want >= 2", len(stream.Sources))
	}
	if !strings.Contains(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("Sources[0].URL = %q; want hls2 (.m3u8) first", stream.Sources[0].URL)
	}
	if !strings.Contains(stream.Sources[1].URL, ".txt") && !strings.Contains(stream.Sources[1].URL, "enterpriseconsulting.sbs") {
		t.Errorf("Sources[1].URL = %q; want hls3 (.txt or enterpriseconsulting.sbs) second", stream.Sources[1].URL)
	}
}
