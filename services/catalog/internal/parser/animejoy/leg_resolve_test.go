package animejoy

import (
	"strings"
	"testing"
)

// --- Sibnet ----------------------------------------------------------------

func TestParseSibnetShell(t *testing.T) {
	path, err := parseSibnetShell(readFixture(t, "sibnet_shell.html"))
	if err != nil {
		t.Fatalf("parseSibnetShell: %v", err)
	}
	const want = "/v/6462ad80a5d17783fef2c185bd5eab61/5263892.mp4"
	if path != want {
		t.Fatalf("parseSibnetShell path: want %q, got %q", want, path)
	}
}

func TestParseSibnetShellErrorsOnGarbage(t *testing.T) {
	if _, err := parseSibnetShell([]byte("<html>no player here</html>")); err == nil {
		t.Fatalf("expected error on garbage sibnet shell")
	}
	if _, err := parseSibnetShell(nil); err == nil {
		t.Fatalf("expected error on nil sibnet shell")
	}
}

// --- AllVideo --------------------------------------------------------------

func TestParseAllVideoFiles(t *testing.T) {
	list, err := parseAllVideoFiles(readFixture(t, "allvideo_incvideo1.html"))
	if err != nil {
		t.Fatalf("parseAllVideoFiles: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("parseAllVideoFiles: want 3 entries, got %d (%+v)", len(list), list)
	}

	byQ := map[string]string{}
	for _, e := range list {
		byQ[e.Quality] = e.URL
	}
	for _, q := range []string{"360p", "720p", "1080p"} {
		if _, ok := byQ[q]; !ok {
			t.Fatalf("parseAllVideoFiles: missing quality %q in %+v", q, list)
		}
	}

	// 720p URL must carry the _720p marker.
	if u := byQ["720p"]; !strings.Contains(u, "726858_720p.mp4") {
		t.Fatalf("720p url %q missing 726858_720p.mp4", u)
	}
	// 1080p is the bare master (no _NNNp suffix), ending .mp4/.
	if u := byQ["1080p"]; !strings.Contains(u, "726858.mp4/") {
		t.Fatalf("1080p url %q missing 726858.mp4/", u)
	}
}

func TestPickBestAllVideo(t *testing.T) {
	list, err := parseAllVideoFiles(readFixture(t, "allvideo_incvideo1.html"))
	if err != nil {
		t.Fatalf("parseAllVideoFiles: %v", err)
	}
	best, ok := pickBestAllVideo(list)
	if !ok {
		t.Fatalf("pickBestAllVideo: no pick")
	}
	if best.Quality != "1080p" {
		t.Fatalf("pickBestAllVideo: want 1080p, got %q", best.Quality)
	}
	if !strings.Contains(best.URL, "get_file") {
		t.Fatalf("pickBestAllVideo url %q missing get_file", best.URL)
	}
	if !strings.HasSuffix(best.URL, ".mp4/") {
		t.Fatalf("pickBestAllVideo url %q does not end .mp4/", best.URL)
	}
	if !strings.Contains(best.URL, "726858.mp4/") {
		t.Fatalf("pickBestAllVideo url %q is not the 1080p master 726858.mp4/", best.URL)
	}
}

func TestPickBestAllVideoEmpty(t *testing.T) {
	if _, ok := pickBestAllVideo(nil); ok {
		t.Fatalf("pickBestAllVideo(nil): want ok=false")
	}
	if _, ok := pickBestAllVideo([]allVideoFile{}); ok {
		t.Fatalf("pickBestAllVideo(empty): want ok=false")
	}
}

func TestParseAllVideoFilesErrorsOnGarbage(t *testing.T) {
	if _, err := parseAllVideoFiles([]byte("<html>no file config</html>")); err == nil {
		t.Fatalf("expected error on garbage allvideo page")
	}
	if _, err := parseAllVideoFiles(nil); err == nil {
		t.Fatalf("expected error on nil allvideo page")
	}
}

func TestDeriveAllVideoReferer(t *testing.T) {
	// The Referer MUST be the get_file URL's own origin (the host that 302s to
	// filevideo1), NOT the fsst embed origin — filevideo1 403s a fsst.online
	// Referer (smoke-tested 2026-06-30). Derived per-resolve so it survives
	// CDN mirror rotation.
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"incvideo1 get_file", "https://www.incvideo1.online/get_file/5/abc/726000/726858/726858.mp4/", "https://www.incvideo1.online/"},
		{"rotated mirror host", "https://incvideo2.online/get_file/9/def/1.mp4/", "https://incvideo2.online/"},
		{"unparseable falls back", "://no-scheme", allVideoFallbackReferer},
		{"empty falls back", "", allVideoFallbackReferer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveAllVideoReferer(c.url); got != c.want {
				t.Fatalf("deriveAllVideoReferer(%q): want %q, got %q", c.url, c.want, got)
			}
		})
	}
	// The fsst embed origin must never be the Referer (the bug we fixed).
	if got := deriveAllVideoReferer("https://www.incvideo1.online/get_file/x/726858.mp4/"); strings.Contains(got, "fsst") {
		t.Fatalf("deriveAllVideoReferer leaked fsst origin: %q", got)
	}
}
