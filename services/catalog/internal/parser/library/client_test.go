package library

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// happyEnvelope returns a libs/httputil-shaped 200 body wrapping an
// EpisodeResponse.
func happyEnvelope() string {
	return `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":737139}}`
}

func TestGetEpisode_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/library/episodes/57466/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, happyEnvelope())
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	got, err := c.GetEpisode(context.Background(), "57466", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil response on 200")
	}
	if got.MinIOURL != "http://minio:9000/raw-library/57466/1/playlist.m3u8" {
		t.Errorf("MinIOURL = %q", got.MinIOURL)
	}
	if got.DurationSec != 10 {
		t.Errorf("DurationSec = %d, want 10", got.DurationSec)
	}
	if got.SizeBytes != 737139 {
		t.Errorf("SizeBytes = %d, want 737139", got.SizeBytes)
	}
}

func TestGetEpisode_EmptyMinIOURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"","duration_sec":0,"size_bytes":0}}`)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.GetEpisode(context.Background(), "57466", 1)
	if err == nil {
		t.Fatal("expected error on empty minio_url, got nil")
	}
	if !strings.Contains(err.Error(), "empty minio_url") {
		t.Errorf("error message = %q, want substring 'empty minio_url'", err.Error())
	}
}

func TestGetEpisode_NotFoundReturnsNilNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	got, err := c.GetEpisode(context.Background(), "57466", 1)
	if err != nil {
		t.Fatalf("404 must return nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("404 must return nil response, got %+v", got)
	}
}

func TestGetEpisode_500ReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.GetEpisode(context.Background(), "57466", 1)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "upstream 500") {
		t.Errorf("error = %q, want substring 'upstream 500'", err.Error())
	}
}

func TestGetEpisode_503ReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.GetEpisode(context.Background(), "57466", 1)
	if err == nil {
		t.Fatal("expected error on 503")
	}
	if !strings.Contains(err.Error(), "upstream 503") {
		t.Errorf("error = %q, want substring 'upstream 503'", err.Error())
	}
}

func TestGetEpisode_TimeoutReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, happyEnvelope())
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 10 * time.Millisecond})
	start := time.Now()
	_, err := c.GetEpisode(context.Background(), "57466", 1)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// http.Client.Timeout surfaces either context.DeadlineExceeded
	// wrapped or a plain "Client.Timeout exceeded" message depending
	// on Go version.
	msg := err.Error()
	if !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(msg, "context deadline exceeded") &&
		!strings.Contains(msg, "Client.Timeout") {
		t.Errorf("error = %q, want timeout-ish substring", msg)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout took %s, want < 500ms (client timeout was 10ms)", elapsed)
	}
}

func TestGetEpisode_InvalidArgs_NoRequestSent(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})

	cases := []struct {
		shikimoriID string
		episode     int
	}{
		{"", 1},
		{"57466", 0},
		{"57466", -1},
	}
	for _, tc := range cases {
		if _, err := c.GetEpisode(context.Background(), tc.shikimoriID, tc.episode); err == nil {
			t.Errorf("expected error for (%q, %d), got nil", tc.shikimoriID, tc.episode)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("expected 0 server hits, got %d", hits.Load())
	}
}

func TestPing_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("ping path = %s, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned %v", err)
	}
}

func TestPing_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected error on 503")
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, happyEnvelope())
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL + "/", Timeout: 2 * time.Second})
	if _, err := c.GetEpisode(context.Background(), "X", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/api/library/episodes/X/1"
	if seenPath != want {
		t.Errorf("path = %q, want %q (trailing slash should be trimmed)", seenPath, want)
	}
}

func TestNewClient_DefaultsTimeoutTo2s(t *testing.T) {
	c := NewClient(Config{APIURL: "http://x"})
	if c.httpClient.Timeout != 2*time.Second {
		t.Errorf("default Timeout = %s, want 2s", c.httpClient.Timeout)
	}
}
