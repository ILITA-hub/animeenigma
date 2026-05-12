// vibeplayer_test.go — GREEN tests for VibePlayerExtractor (regex-only).
//
// SCRAPER-9ANI-03 (SSRF gate at Matches()) and SCRAPER-9ANI-04 (Extract from
// captured offline golden). Tests use the package-private rewriteToSrv
// RoundTripper (defined in packed_common_test.go) to keep Matches() validating
// against the real allowlisted host (vibeplayer.site) while routing the
// underlying TCP socket to a local httptest server serving the golden.
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

// vibePlayerGolden resolves the vibeplayer_embed.html golden captured in
// Plan 18-01 Task 3 (path: services/scraper/testdata/gogoanime/vibeplayer_embed.html).
func vibePlayerGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "gogoanime", "vibeplayer_embed.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestVibePlayer_Matches_RejectsSubdomainImposters verifies SCRAPER-9ANI-03's
// SSRF gate: VibePlayerExtractor.Matches() must reject impostor hosts that
// merely contain "vibeplayer.site" as a substring (e.g. evilvibeplayer.site)
// — only host equality OR strict subdomain (HasSuffix host, "."+known) match.
func TestVibePlayer_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Parallel()
	e := NewVibePlayerExtractor()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://vibeplayer.site/abc", true},          // exact
		{"https://sub.vibeplayer.site/abc", true},      // strict subdomain
		{"HTTPS://VIBEPLAYER.SITE/abc", true},          // case-insensitive
		{"http://vibeplayer.site/abc", true},           // http allowed
		{"https://evilvibeplayer.site/abc", false},     // impostor — no dot boundary
		{"https://vibeplayer.com/abc", false},          // wrong TLD
		{"https://vibeplayer.site.evil.com", false},    // suffix attack
		{"ftp://vibeplayer.site/abc", false},           // wrong scheme
		{"file:///vibeplayer.site/passwd", false},      // wrong scheme
		{"https:///abc", false},                        // empty host
		{"", false},                                    // empty URL
		{"://no-scheme", false},                        // malformed
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

// TestVibePlayer_Name pins the stable identifier emitted in logs / metrics.
func TestVibePlayer_Name(t *testing.T) {
	t.Parallel()
	if got := NewVibePlayerExtractor().Name(); got != "vibeplayer" {
		t.Errorf("Name() = %q; want %q", got, "vibeplayer")
	}
}

// TestVibePlayer_Extract_FromGolden verifies SCRAPER-9ANI-04: vibeplayer
// extractor parses `const src = "https://...m3u8"` from the captured
// vibeplayer_embed.html golden via regex (no goja — regex-only path).
//
// The rewriteToSrv RoundTripper preserves the allowlisted vibeplayer.site
// host on the Request URL (so Matches() succeeds) while routing the actual
// HTTP socket to the local httptest server.
func TestVibePlayer_Extract_FromGolden(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(vibePlayerGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewVibePlayerExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	stream, err := e.Extract(
		context.Background(),
		"https://vibeplayer.site/embed-aac165bfc862642b",
		http.Header{},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) == 0 {
		t.Fatalf("Extract returned empty stream: %+v", stream)
	}
	if !strings.HasSuffix(stream.Sources[0].URL, ".m3u8") {
		t.Errorf("source URL = %q; want suffix .m3u8", stream.Sources[0].URL)
	}
	if !strings.Contains(stream.Sources[0].URL, "vibeplayer.site") {
		t.Errorf("source URL = %q; want vibeplayer.site host", stream.Sources[0].URL)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("source type = %q; want hls", stream.Sources[0].Type)
	}
	if stream.Headers["Referer"] != "https://vibeplayer.site/" {
		t.Errorf("Referer header = %q; want https://vibeplayer.site/", stream.Headers["Referer"])
	}
	// The captured golden has `const subtitle = ""` (empty) — Tracks must be empty.
	if len(stream.Tracks) != 0 {
		t.Errorf("Tracks len = %d; want 0 (golden has empty subtitle const)", len(stream.Tracks))
	}
}

// TestVibePlayer_Extract_NoSrc serves a 200 with no `const src=`. Extract
// must wrap-error with ErrExtractFailed (not ErrProviderDown — the server
// answered) and increment parser_zero_match_total.
func TestVibePlayer_Extract_NoSrc(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>no const src here</body></html>`))
	}))
	t.Cleanup(srv.Close)

	e := NewVibePlayerExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://vibeplayer.site/embed-missing",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrExtractFailed")
	}
}

// TestVibePlayer_Extract_SubtitleURLValidation verifies CR-01 from the Phase
// 18 review: VibePlayerExtractor MUST drop any captured `const subtitle =
// "..."` payload that fails the (shape, host) allowlist gates. The stream
// itself is still returned — only the suspicious Track is omitted, so a
// hostile or compromised vibeplayer.site cannot inject javascript:, file:,
// data:, ftp:, or attacker-controlled subtitle URLs into the player.
//
// Each case constructs a wrapper page that emits a valid m3u8 src AND a
// crafted subtitle const; the test asserts that the m3u8 survives but the
// Tracks slice is empty for every attacker payload, and non-empty for the
// known-good cdn.cimovix.store / vibeplayer.site captions URL.
func TestVibePlayer_Extract_SubtitleURLValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		subtitle  string
		wantTrack bool
	}{
		{
			name:      "javascript_scheme_rejected",
			subtitle:  "javascript:alert(1)",
			wantTrack: false,
		},
		{
			name:      "file_scheme_rejected",
			subtitle:  "file:///etc/passwd",
			wantTrack: false,
		},
		{
			name:      "data_scheme_rejected",
			subtitle:  "data:text/plain;base64,AAAA",
			wantTrack: false,
		},
		{
			name:      "ftp_scheme_rejected",
			subtitle:  "ftp://evil.example.com/sub.vtt",
			wantTrack: false,
		},
		{
			name:      "https_offhost_rejected",
			subtitle:  "https://evil.example.com/sub.vtt",
			wantTrack: false,
		},
		{
			name:      "https_offhost_suffix_attack_rejected",
			subtitle:  "https://vibeplayer.site.evil.com/sub.vtt",
			wantTrack: false,
		},
		{
			name:      "https_wrong_extension_rejected",
			subtitle:  "https://cdn.cimovix.store/sub.exe",
			wantTrack: false,
		},
		{
			name:      "https_no_extension_rejected",
			subtitle:  "https://cdn.cimovix.store/sub",
			wantTrack: false,
		},
		{
			name:      "valid_cimovix_vtt_accepted",
			subtitle:  "https://cdn.cimovix.store/foo/sub.vtt",
			wantTrack: true,
		},
		{
			name:      "valid_cimovix_srt_accepted",
			subtitle:  "https://cdn.cimovix.store/foo/sub.srt",
			wantTrack: true,
		},
		{
			name:      "valid_cimovix_ass_accepted",
			subtitle:  "https://cdn.cimovix.store/foo/sub.ass",
			wantTrack: true,
		},
		{
			name:      "valid_vibeplayer_subdomain_accepted",
			subtitle:  "https://sub.vibeplayer.site/foo.vtt",
			wantTrack: true,
		},
		{
			name:      "valid_with_query_string_accepted",
			subtitle:  "https://cdn.cimovix.store/sub.vtt?expires=123",
			wantTrack: true,
		},
		{
			name:      "empty_subtitle_no_track",
			subtitle:  "",
			wantTrack: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page := `<html><body><script>
const src = "https://vibeplayer.site/public/stream/abc/master.m3u8";
const subtitle = "` + tc.subtitle + `";
</script></body></html>`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(page))
			}))
			t.Cleanup(srv.Close)

			e := NewVibePlayerExtractor()
			e.http = &http.Client{
				Transport: &rewriteToSrv{srvURL: srv.URL},
				Timeout:   5 * time.Second,
			}
			stream, err := e.Extract(
				context.Background(),
				"https://vibeplayer.site/embed-test",
				http.Header{},
			)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if stream == nil || len(stream.Sources) == 0 {
				t.Fatalf("Extract returned empty stream: %+v", stream)
			}
			// The m3u8 must always survive — defence is about Tracks only.
			if !strings.HasSuffix(stream.Sources[0].URL, ".m3u8") {
				t.Errorf("source URL = %q; want .m3u8", stream.Sources[0].URL)
			}
			gotTrack := len(stream.Tracks) > 0
			if gotTrack != tc.wantTrack {
				t.Errorf("len(Tracks) = %d; want_track = %v; subtitle = %q",
					len(stream.Tracks), tc.wantTrack, tc.subtitle)
			}
			if tc.wantTrack && len(stream.Tracks) > 0 {
				if stream.Tracks[0].File != tc.subtitle {
					t.Errorf("Tracks[0].File = %q; want %q",
						stream.Tracks[0].File, tc.subtitle)
				}
			}
		})
	}
}
