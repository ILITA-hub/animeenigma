package eighteenanime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestBaseSlugAndEpisode
// ---------------------------------------------------------------------------

func TestBaseSlugAndEpisode(t *testing.T) {
	cases := []struct {
		slug        string
		wantBase    string
		wantEpisode int
	}{
		{
			slug:        "1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2",
			wantBase:    "jk-to-inkou-kyoushi-4-feat-ero-giin-sensei",
			wantEpisode: 2,
		},
		{
			slug:        "2172-inkou-kyoushi-no-saimin-seikatsu-shidouroku-2",
			wantBase:    "inkou-kyoushi-no-saimin-seikatsu-shidouroku",
			wantEpisode: 2,
		},
		{
			// slug with no episode suffix → episode defaults to 1
			slug:        "999-some-series-title",
			wantBase:    "some-series-title",
			wantEpisode: 1,
		},
	}

	for _, tc := range cases {
		base, ep := baseSlugAndEpisode(tc.slug)
		if base != tc.wantBase {
			t.Errorf("baseSlugAndEpisode(%q) base = %q, want %q", tc.slug, base, tc.wantBase)
		}
		if ep != tc.wantEpisode {
			t.Errorf("baseSlugAndEpisode(%q) episode = %d, want %d", tc.slug, ep, tc.wantEpisode)
		}
	}
}

// ---------------------------------------------------------------------------
// TestListEpisodes — httptest server serves the search fixture
// ---------------------------------------------------------------------------

func TestListEpisodes(t *testing.T) {
	data, err := os.ReadFile("testdata/search_results.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := NewClient()
	c.searchBase = srv.URL

	eps, err := c.ListEpisodes(context.Background(), "JK to Inkou Kyoushi 4 feat Ero Giin Sensei")
	if err != nil {
		t.Fatalf("ListEpisodes error: %v", err)
	}

	if len(eps) == 0 {
		t.Fatal("expected at least one episode, got 0")
	}

	// All returned episodes must share the same base slug.
	wantBase := "jk-to-inkou-kyoushi-4-feat-ero-giin-sensei"
	for _, ep := range eps {
		base, _ := baseSlugAndEpisode(ep.Slug)
		if base != wantBase {
			t.Errorf("unexpected base slug %q in episode %+v; want base %q", base, ep, wantBase)
		}
	}

	// Episodes must be sorted ascending by Number.
	for i := 1; i < len(eps); i++ {
		if eps[i].Number < eps[i-1].Number {
			t.Errorf("episodes not sorted: eps[%d].Number=%d < eps[%d].Number=%d",
				i, eps[i].Number, i-1, eps[i-1].Number)
		}
	}

	// The "inkou-kyoushi-no-saimin-seikatsu-shidouroku" series must NOT appear.
	for _, ep := range eps {
		if strings.Contains(ep.Slug, "saimin-seikatsu-shidouroku") {
			t.Errorf("shidouroku series leaked into jk-feat result: slug=%q", ep.Slug)
		}
	}

	t.Logf("ListEpisodes returned %d episodes for %q", len(eps), wantBase)
	for _, ep := range eps {
		t.Logf("  ep %d  slug=%s  url=%s", ep.Number, ep.Slug, ep.URL)
	}
}

// ---------------------------------------------------------------------------
// TestResolveStreamFailover
// ---------------------------------------------------------------------------

func TestResolveStreamFailover(t *testing.T) {
	mp4UploadBody, err := os.ReadFile("testdata/embed_mp4upload.html")
	if err != nil {
		t.Fatalf("read mp4upload fixture: %v", err)
	}

	// Track which paths were called.
	var calledPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPaths = append(calledPaths, r.URL.Path)
		switch r.URL.Path {
		case "/mp4upload/embed-dead.html":
			// First mirror: returns 404 — simulates a broken mirror.
			w.WriteHeader(http.StatusNotFound)
		case "/mp4upload/embed-good.html":
			// Second mirror: returns the mp4upload fixture.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mp4UploadBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Both mirrors contain "mp4upload" in their URLs so they both pass supportedMirrors.
	// supportedMirrors preserves the order within each host group, so the dead mirror
	// appears first and the good one second — exercising the failover path.
	mirrors := []Mirror{
		{Link: srv.URL + "/mp4upload/embed-dead.html", Quality: "FullHD"},
		{Link: srv.URL + "/mp4upload/embed-good.html", Quality: "FullHD"},
	}

	c := NewClient()
	src, err := c.resolveStream(context.Background(), mirrors)
	if err != nil {
		t.Fatalf("resolveStream error: %v", err)
	}
	if src == nil {
		t.Fatal("expected non-nil ExtractedSource, got nil")
	}
	if src.URL == "" {
		t.Error("ExtractedSource.URL is empty, want a non-empty MP4 URL")
	}

	// Confirm the first (bad) path was actually attempted before the good one.
	foundBad := false
	for _, p := range calledPaths {
		if p == "/mp4upload/embed-dead.html" {
			foundBad = true
			break
		}
	}
	if !foundBad {
		t.Error("expected the failing mirror to be attempted before failover, but /mp4upload/embed-dead.html was never called")
	}

	t.Logf("resolveStream succeeded with URL=%q after failover", src.URL)
}
