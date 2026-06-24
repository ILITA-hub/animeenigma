package anime365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleASS = "[Script Info]\nTitle: x\nScriptType: v4.00+\n\n[Events]\n" +
	"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Привет\n"

func TestDownloadSubtitle_PrefersASS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/episodeTranslations/") {
			_, _ = w.Write([]byte(sampleASS))
			return
		}
		t.Fatalf("unexpected path %s (should not fall back)", r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	body, format, err := c.DownloadSubtitle(context.Background(), 5819457)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "ass" {
		t.Fatalf("format = %q, want ass", format)
	}
	if !strings.Contains(string(body), "Dialogue:") {
		t.Fatalf("body missing Dialogue: %q", string(body))
	}
}

func TestDownloadSubtitle_FallsBackToVTTWhenASSMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/episodeTranslations/"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/translations/vtt/"):
			_, _ = w.Write([]byte("WEBVTT\n\n00:01.000 --> 00:02.000\nПривет\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	body, format, err := c.DownloadSubtitle(context.Background(), 1)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "vtt" {
		t.Fatalf("format = %q, want vtt", format)
	}
	if !strings.HasPrefix(string(body), "WEBVTT") {
		t.Fatalf("body not vtt: %q", string(body))
	}
}

func TestDownloadSubtitle_FallsBackWhenASSMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/episodeTranslations/"):
			_, _ = w.Write([]byte("<html>paywall</html>")) // 200 but not ASS
		case strings.HasPrefix(r.URL.Path, "/translations/vtt/"):
			_, _ = w.Write([]byte("WEBVTT\n\n00:01.000 --> 00:02.000\nПривет\n"))
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	_, format, err := c.DownloadSubtitle(context.Background(), 1)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "vtt" {
		t.Fatalf("format = %q, want vtt (malformed ASS should fall back)", format)
	}
}

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
