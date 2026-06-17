package library

import (
	"context"
	"encoding/json"
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

func TestListEpisodes_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/library/episodes/54974" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[{"episode_number":1,"minio_url":"http://minio:9000/raw-library/54974/1/playlist.m3u8","duration_sec":1450},{"episode_number":2,"minio_url":"http://minio:9000/raw-library/54974/2/playlist.m3u8"}]}}`)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	got, err := c.ListEpisodes(context.Background(), "54974")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].EpisodeNumber != 1 || got[0].MinIOURL == "" || got[0].DurationSec != 1450 {
		t.Errorf("ep0 = %+v", got[0])
	}
	if got[1].EpisodeNumber != 2 {
		t.Errorf("ep1 number = %d, want 2", got[1].EpisodeNumber)
	}
}

func TestListEpisodes_EmptyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[]}}`)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	got, err := c.ListEpisodes(context.Background(), "54974")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestListEpisodes_5xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	if _, err := c.ListEpisodes(context.Background(), "54974"); err == nil {
		t.Fatal("expected error on 5xx, got nil")
	}
}

func TestListEpisodes_EmptyShikimoriID(t *testing.T) {
	c := NewClient(Config{APIURL: "http://unused", Timeout: time.Second})
	if _, err := c.ListEpisodes(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty shikimori_id")
	}
}

// ---- Phase 08-03: best-effort serve-signal producers ----

// decodeJSONBody is a tiny helper for the internal-endpoint tests: reads
// the POST body into a generic map so assertions can inspect mal_id /
// episode / reason without a typed struct.
func decodeJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return got
}

func TestRecordFetch_PostsFetchPathWithBody(t *testing.T) {
	var seenPath, seenMethod, seenCT string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenMethod = r.Method
		seenCT = r.Header.Get("Content-Type")
		body = decodeJSONBody(t, r)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	if err := c.RecordFetch(context.Background(), "57466", 3); err != nil {
		t.Fatalf("RecordFetch returned %v, want nil on 200", err)
	}
	if seenPath != "/internal/library/autocache/fetch" {
		t.Errorf("path = %q, want /internal/library/autocache/fetch", seenPath)
	}
	if seenMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", seenMethod)
	}
	if !strings.Contains(seenCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", seenCT)
	}
	if body["mal_id"] != "57466" {
		t.Errorf("mal_id = %v, want 57466", body["mal_id"])
	}
	// JSON numbers decode to float64.
	if ep, ok := body["episode"].(float64); !ok || int(ep) != 3 {
		t.Errorf("episode = %v, want 3", body["episode"])
	}
	if _, present := body["reason"]; present {
		t.Errorf("fetch body should not carry a reason, got %v", body["reason"])
	}
}

func TestRecordDemand_PostsDemandPathWithReason(t *testing.T) {
	var seenPath string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		body = decodeJSONBody(t, r)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	if err := c.RecordDemand(context.Background(), "57466", 7, "backfill"); err != nil {
		t.Fatalf("RecordDemand returned %v, want nil on 200", err)
	}
	if seenPath != "/internal/library/autocache/demand" {
		t.Errorf("path = %q, want /internal/library/autocache/demand", seenPath)
	}
	if body["mal_id"] != "57466" {
		t.Errorf("mal_id = %v, want 57466", body["mal_id"])
	}
	if ep, ok := body["episode"].(float64); !ok || int(ep) != 7 {
		t.Errorf("episode = %v, want 7", body["episode"])
	}
	if body["reason"] != "backfill" {
		t.Errorf("reason = %v, want backfill", body["reason"])
	}
}

func TestRecordFetch_500ReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	err := c.RecordFetch(context.Background(), "57466", 1)
	if err == nil {
		t.Fatal("expected wrapped error on 500 so the caller can log it")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want substring '500'", err.Error())
	}
}

func TestRecordDemand_500ReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(Config{APIURL: srv.URL, Timeout: 2 * time.Second})
	err := c.RecordDemand(context.Background(), "57466", 1, "backfill")
	if err == nil {
		t.Fatal("expected wrapped error on 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %q, want substring '503'", err.Error())
	}
}
