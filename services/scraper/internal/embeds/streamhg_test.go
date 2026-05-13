// streamhg_test.go — GREEN tests for StreamHGExtractor (Dean-Edwards packer).
//
// SCRAPER-9ANI-03 (SSRF gate) + SCRAPER-9ANI-04 (Extract from offline golden).
// Reuses the rewriteToSrv RoundTripper defined in packed_common_test.go to
// keep Matches() validating against the real allowlisted host (otakuhg.site)
// while routing the actual TCP socket to a local httptest server.
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

// streamhgGolden resolves the streamhg_packed.html golden captured in Plan
// 18-01 Task 3 (path: services/scraper/testdata/gogoanime/streamhg_packed.html).
func streamhgGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "streamhg_packed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestStreamHG_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate at the StreamHG-specific allowlist.
func TestStreamHG_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Parallel()
	e := NewStreamHGExtractor()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://otakuhg.site/abc", true},
		{"https://cdn.otakuhg.site/abc", true},
		{"HTTPS://OTAKUHG.SITE/abc", true},
		{"https://evilotakuhg.site/abc", false},
		{"https://otakuhg.com/abc", false},
		{"https://otakuhg.site.evil.com", false},
		{"ftp://otakuhg.site/abc", false},
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

// TestStreamHG_Name pins the stable identifier emitted in logs / metrics.
func TestStreamHG_Name(t *testing.T) {
	t.Parallel()
	if got := NewStreamHGExtractor().Name(); got != "streamhg" {
		t.Errorf("Name() = %q; want %q", got, "streamhg")
	}
}

// TestStreamHG_Extract_FromGolden verifies SCRAPER-9ANI-04: StreamHG
// extracts the `"hls2":"...m3u8..."` URL from the Dean-Edwards packed body in
// the captured streamhg_packed.html golden, using the shared packedExtractor
// pipeline + the rewriteToSrv RoundTripper.
func TestStreamHG_Extract_FromGolden(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(streamhgGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewStreamHGExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://otakuhg.site/d/tt7s8h2dhneqk7",
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
	if stream.Headers["Referer"] != "https://otakuhg.site/" {
		t.Errorf("Referer header = %q; want https://otakuhg.site/", stream.Headers["Referer"])
	}
	// StreamHG signs URLs with an `e=` expiry query param — Plan 18-02's
	// cache TTL parser keys on this. The golden captures a real signed URL.
	if !strings.Contains(stream.Sources[0].URL, "e=") {
		t.Errorf("source URL = %q; want substring 'e=' (signed-expiry param needed by Plan 18-02 cache TTL)", stream.Sources[0].URL)
	}
}

// TestStreamHG_MultiURL_FromGolden — Plan 22-01 Task 1. Asserts the streamhg
// extractor returns BOTH the hls2 (signed .m3u8) and hls3 (.txt) URLs from
// the captured golden, ordered hls2-first. Without multi-URL extraction the
// downstream cold-path Sources iteration is dead code.
func TestStreamHG_MultiURL_FromGolden(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(streamhgGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewStreamHGExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://otakuhg.site/d/tt7s8h2dhneqk7",
		http.Header{},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) < 2 {
		t.Fatalf("len(Sources) < 2; got %d sources; full=%+v", len(stream.Sources), stream)
	}
	// hls2 first.
	if !strings.Contains(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("Sources[0].URL = %q; want substring .m3u8 (hls2 first)", stream.Sources[0].URL)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("Sources[0].Type = %q; want hls", stream.Sources[0].Type)
	}
	// hls3 — exists somewhere in Sources (positionally second after hls2).
	foundHls3 := false
	for _, s := range stream.Sources {
		if strings.Contains(s.URL, "professionalimage.cyou") && strings.HasSuffix(s.URL, ".txt") {
			foundHls3 = true
			if s.Type != "hls" {
				t.Errorf("hls3 Source Type = %q; want hls", s.Type)
			}
			break
		}
	}
	if !foundHls3 {
		t.Errorf("no Source contains professionalimage.cyou *.txt URL; sources=%+v", stream.Sources)
	}
	if stream.Headers["Referer"] != "https://otakuhg.site/" {
		t.Errorf("Referer header = %q; want https://otakuhg.site/", stream.Headers["Referer"])
	}
}

// TestStreamHG_MultiURL_Order — pins the ordering invariant: Sources[0] is the
// signed m3u8 (hls2), Sources[1] is the unsigned txt (hls3).
func TestStreamHG_MultiURL_Order(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(streamhgGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewStreamHGExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(context.Background(), "https://otakuhg.site/d/tt7s8h2dhneqk7", http.Header{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(stream.Sources) < 2 {
		t.Fatalf("len(Sources) = %d; want >= 2", len(stream.Sources))
	}
	if !strings.Contains(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("Sources[0].URL = %q; want hls2 (.m3u8) first", stream.Sources[0].URL)
	}
	if !strings.Contains(stream.Sources[1].URL, ".txt") && !strings.Contains(stream.Sources[1].URL, "professionalimage.cyou") {
		t.Errorf("Sources[1].URL = %q; want hls3 (.txt or professionalimage.cyou) second", stream.Sources[1].URL)
	}
}

// TestStreamHG_ExtractURL_HasExpiryQuery is a focused regression test that
// asserts the extracted URL carries the `e=<digits>` expiry parameter
// (matching the contract Plan 18-02's TTL parser depends on). If StreamHG
// ever stops signing URLs and the cache layer assumes the param is present,
// this fires immediately.
func TestStreamHG_ExtractURL_HasExpiryQuery(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(streamhgGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewStreamHGExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   10 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://otakuhg.site/d/tt7s8h2dhneqk7",
		http.Header{},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(stream.Sources[0].URL, "e=") {
		t.Errorf("extracted URL %q lacks &e= expiry param (Plan 18-02 cache TTL depends on it)", stream.Sources[0].URL)
	}
}
