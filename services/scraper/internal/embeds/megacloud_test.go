package embeds

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Compile-time assertion that MegacloudClient satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*MegacloudClient)(nil)

// TestMegacloudClient_MatchesKnownHosts asserts that every host the Aniyomi
// extension and some upstreams rotate through is recognized as a megacloud
// embed, and that unrelated hosts (kwik, animepahe, generic example.com with
// "megacloud" in the path) are NOT mistaken for one.
func TestMegacloudClient_MatchesKnownHosts(t *testing.T) {
	t.Parallel()
	mc := NewMegacloudClient("http://megacloud-extractor:3200", 0)

	matches := []string{
		"https://megacloud.tv/embed-2/abc",
		"https://megacloud.blog/embed/foo",
		"https://megacloud.club/embed/x",
		"https://megaup.live/e/foo",
		"https://megaup.cc/e/foo",
		// Subdomains of a known host
		"https://cdn.megacloud.tv/x",
		"https://www.megacloud.blog/embed/foo",
	}
	for _, u := range matches {
		u := u
		t.Run("match_"+u, func(t *testing.T) {
			t.Parallel()
			if !mc.Matches(u) {
				t.Errorf("Matches(%q) = false; want true", u)
			}
		})
	}

	nonMatches := []string{
		"https://kwik.cx/e/x",
		"https://animepahe.ru/play/x",
		"https://example.com/megacloud-imposter", // megacloud in PATH, not host
		"https://example.com/path?q=megacloud.tv",
		"not a url",
	}
	for _, u := range nonMatches {
		u := u
		t.Run("nomatch_"+u, func(t *testing.T) {
			t.Parallel()
			if mc.Matches(u) {
				t.Errorf("Matches(%q) = true; want false", u)
			}
		})
	}
}

// TestMegacloudClient_Extract_Success spins up an httptest sidecar that returns
// the canonical sidecar JSON and asserts the returned *Stream contains the same
// values (with field-name translation: sidecar `url`→Source.URL, `lang`→Track.Label).
func TestMegacloudClient_Extract_Success(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"sources": []map[string]any{
			{"url": "https://cdn.example.com/master.m3u8", "type": "hls", "isM3U8": true},
		},
		"tracks": []map[string]any{
			{"url": "https://cdn.example.com/en.vtt", "lang": "English", "default": true},
		},
		"intro": map[string]any{"start": 5, "end": 95},
		"outro": map[string]any{"start": 1380, "end": 1410},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	mc := NewMegacloudClient(srv.URL, 5*time.Second)
	stream, err := mc.Extract(context.Background(), "https://megacloud.tv/embed-2/abc?k=1", nil)
	if err != nil {
		t.Fatalf("Extract: unexpected err: %v", err)
	}
	if stream == nil {
		t.Fatal("Extract returned nil stream")
	}
	if len(stream.Sources) != 1 {
		t.Fatalf("Sources len = %d; want 1", len(stream.Sources))
	}
	if stream.Sources[0].URL != "https://cdn.example.com/master.m3u8" {
		t.Errorf("Source.URL = %q; want canonical m3u8", stream.Sources[0].URL)
	}
	if stream.Sources[0].Type != "hls" {
		t.Errorf("Source.Type = %q; want hls", stream.Sources[0].Type)
	}
	if len(stream.Tracks) != 1 {
		t.Fatalf("Tracks len = %d; want 1", len(stream.Tracks))
	}
	if stream.Tracks[0].File != "https://cdn.example.com/en.vtt" {
		t.Errorf("Track.File = %q; want canonical vtt", stream.Tracks[0].File)
	}
	if stream.Tracks[0].Label != "English" {
		t.Errorf("Track.Label = %q; want English", stream.Tracks[0].Label)
	}
	if !stream.Tracks[0].Default {
		t.Errorf("Track.Default = false; want true")
	}
	if stream.Intro == nil || stream.Intro.End != 95 {
		t.Errorf("Intro = %+v; want End=95", stream.Intro)
	}
	if stream.Outro == nil || stream.Outro.End != 1410 {
		t.Errorf("Outro = %+v; want End=1410", stream.Outro)
	}
}

// TestMegacloudClient_Extract_SidecarError asserts that a 500 + JSON-error
// body is mapped to a wrapped ErrExtractFailed whose cause string contains
// the sidecar's error message.
func TestMegacloudClient_Extract_SidecarError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "upstream offline"})
	}))
	t.Cleanup(srv.Close)

	mc := NewMegacloudClient(srv.URL, 5*time.Second)
	_, err := mc.Extract(context.Background(), "https://megacloud.tv/embed-2/abc", nil)
	if err == nil {
		t.Fatal("Extract: want err on sidecar 500, got nil")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is match ErrExtractFailed", err)
	}
	if !strings.Contains(err.Error(), "upstream offline") {
		t.Errorf("err.Error() = %q; want substring %q", err.Error(), "upstream offline")
	}
}

// TestMegacloudClient_Extract_SendsURLParam asserts that the URL passed to
// Extract is URL-encoded into the ?url= query param verbatim.
func TestMegacloudClient_Extract_SendsURLParam(t *testing.T) {
	t.Parallel()

	embed := "https://megacloud.tv/embed-2/abc?z=1&y=2"
	var receivedRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRaw = r.URL.Query().Get("url")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sources":[],"tracks":[],"intro":{"start":0,"end":0},"outro":{"start":0,"end":0}}`))
	}))
	t.Cleanup(srv.Close)

	mc := NewMegacloudClient(srv.URL, 5*time.Second)
	if _, err := mc.Extract(context.Background(), embed, nil); err != nil {
		t.Fatalf("Extract: unexpected err: %v", err)
	}
	if receivedRaw != embed {
		t.Errorf("sidecar received url=%q; want %q", receivedRaw, embed)
	}

	// Sanity: also check the path was /extract.
	srv2URL, _ := url.Parse(srv.URL)
	if srv2URL == nil {
		t.Fatal("could not parse srv URL")
	}
}

// TestMegacloudClient_Extract_HonorsContextCancel asserts that a short
// context deadline is honored even when the sidecar hangs.
func TestMegacloudClient_Extract_HonorsContextCancel(t *testing.T) {
	t.Parallel()

	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hang // block forever
	}))
	// Cleanup ordering: close(hang) BEFORE srv.Close so the handler goroutine
	// is released and srv.Close doesn't block. t.Cleanup runs LIFO.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hang) })

	mc := NewMegacloudClient(srv.URL, 30*time.Second) // long client timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := mc.Extract(ctx, "https://megacloud.tv/embed-2/abc", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Extract: want err on ctx cancel, got nil")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want wrapped as ErrExtractFailed", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Extract took %v; should bail within ~50ms+overhead", elapsed)
	}
}

// TestMegacloudClient_Extract_NoMatchingURL_StillCallsSidecar documents that
// Extract does NOT pre-filter on URL pattern: the caller is expected to gate
// via Find() first. The sidecar is responsible for any 404 / "wrong page"
// handling on its side.
func TestMegacloudClient_Extract_NoMatchingURL_StillCallsSidecar(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sources":[],"tracks":[],"intro":{"start":0,"end":0},"outro":{"start":0,"end":0}}`))
	}))
	t.Cleanup(srv.Close)

	mc := NewMegacloudClient(srv.URL, 5*time.Second)
	// Pass a URL that doesn't match (.com is not a known megacloud host).
	if _, err := mc.Extract(context.Background(), "https://random.example.com/x", nil); err != nil {
		t.Fatalf("Extract: unexpected err: %v", err)
	}
	if !called {
		t.Error("sidecar was NOT called; Extract should not pre-filter on URL pattern")
	}
}

// TestMegacloudClient_Name locks the extractor name to the lowercase id
// "megacloud" used in logs and observability labels.
func TestMegacloudClient_Name(t *testing.T) {
	t.Parallel()
	mc := NewMegacloudClient("http://x", 0)
	if mc.Name() != "megacloud" {
		t.Errorf("Name() = %q; want %q", mc.Name(), "megacloud")
	}
}

// TestMegacloudClient_Extract_PassesCallerHeaders verifies that headers
// passed by the caller (for example Referer) reach the
// sidecar request. The sidecar's internal Referer to the upstream embed is
// independent of this — caller headers are headers TO the sidecar.
func TestMegacloudClient_Extract_PassesCallerHeaders(t *testing.T) {
	t.Parallel()

	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sources":[],"tracks":[],"intro":{"start":0,"end":0},"outro":{"start":0,"end":0}}`))
	}))
	t.Cleanup(srv.Close)

	mc := NewMegacloudClient(srv.URL, 5*time.Second)
	h := http.Header{}
	h.Set("Referer", "https://aniwatchtv.to/")
	h.Set("X-Custom", "foo")

	if _, err := mc.Extract(context.Background(), "https://megacloud.tv/embed-2/abc", h); err != nil {
		t.Fatalf("Extract: unexpected err: %v", err)
	}
	if got.Get("Referer") != "https://aniwatchtv.to/" {
		t.Errorf("sidecar req Referer = %q; want aniwatchtv.to", got.Get("Referer"))
	}
	if got.Get("X-Custom") != "foo" {
		t.Errorf("sidecar req X-Custom = %q; want foo", got.Get("X-Custom"))
	}
}
