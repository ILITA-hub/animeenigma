package scraper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestClient_GetEpisodes_BuildsURL verifies the thin client targets
// /scraper/episodes with the expected mal_id + prefer query params.
func TestClient_GetEpisodes_BuildsURL(t *testing.T) {
	var capturedPath, capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"not-yet-implemented","phase":15}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	status, body, err := c.GetEpisodes(context.Background(), 12345, "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if capturedPath != "/scraper/episodes" {
		t.Fatalf("path = %q, want /scraper/episodes", capturedPath)
	}
	if !strings.Contains(capturedQuery, "mal_id=12345") {
		t.Errorf("query = %q, missing mal_id=12345", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "prefer=animepahe") {
		t.Errorf("query = %q, missing prefer=animepahe", capturedQuery)
	}
	if !strings.Contains(string(body), "not-yet-implemented") {
		t.Errorf("body = %q, missing not-yet-implemented", string(body))
	}
}

// TestClient_GetEpisodes_Returns503Verbatim — a 503 is a legitimate
// response, not an error. Status + body forwarded verbatim, err==nil.
func TestClient_GetEpisodes_Returns503Verbatim(t *testing.T) {
	want := `{"error":"not-yet-implemented","phase":15}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	status, body, err := c.GetEpisodes(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("503 must not be an error, got %v", err)
	}
	if status != 503 {
		t.Fatalf("status = %d, want 503", status)
	}
	if string(body) != want {
		t.Errorf("body = %q, want %q", string(body), want)
	}
}

// TestClient_GetEpisodes_Returns500_PropagatesAsError — 5xx that isn't 503
// means the scraper itself is unhealthy; surface it as an error so the
// catalog handler can map to 502 (rather than transparently passing through).
func TestClient_GetEpisodes_Returns500_PropagatesAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kaboom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	status, body, err := c.GetEpisodes(context.Background(), 1, "")
	if err == nil {
		t.Fatalf("expected error for 500, got nil")
	}
	if !errors.Is(err, ErrScraperUpstream) {
		t.Errorf("err = %v, want errors.Is(err, ErrScraperUpstream)", err)
	}
	if status != 500 {
		t.Errorf("status = %d, want 500", status)
	}
	if len(body) == 0 {
		t.Error("body should still be returned even on 5xx error")
	}
}

// TestClient_GetServers_BuildsURL verifies servers endpoint URL + params.
func TestClient_GetServers_BuildsURL(t *testing.T) {
	var capturedPath, capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"not-yet-implemented","phase":15}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	_, _, err := c.GetServers(context.Background(), 42, "ep-1", "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/scraper/servers" {
		t.Fatalf("path = %q, want /scraper/servers", capturedPath)
	}
	if !strings.Contains(capturedQuery, "mal_id=42") {
		t.Errorf("query = %q, missing mal_id=42", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "episode=ep-1") {
		t.Errorf("query = %q, missing episode=ep-1", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "prefer=animepahe") {
		t.Errorf("query = %q, missing prefer=animepahe", capturedQuery)
	}
}

// TestClient_GetStream_BuildsURL verifies stream endpoint URL + all params.
func TestClient_GetStream_BuildsURL(t *testing.T) {
	var capturedPath, capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"not-yet-implemented","phase":15}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	_, _, err := c.GetStream(context.Background(), 7, "ep-2", "srv-1", "sub", "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/scraper/stream" {
		t.Fatalf("path = %q, want /scraper/stream", capturedPath)
	}
	for _, want := range []string{"mal_id=7", "episode=ep-2", "server=srv-1", "category=sub", "prefer=animepahe"} {
		if !strings.Contains(capturedQuery, want) {
			t.Errorf("query = %q, missing %q", capturedQuery, want)
		}
	}
}

// TestClient_GetHealth_BuildsURL — /scraper/health has no query params.
func TestClient_GetHealth_BuildsURL(t *testing.T) {
	var capturedPath, capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"data":{"providers":{}}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	status, body, err := c.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/scraper/health" {
		t.Fatalf("path = %q, want /scraper/health", capturedPath)
	}
	if capturedQuery != "" {
		t.Errorf("query = %q, want empty for health endpoint", capturedQuery)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if !strings.Contains(string(body), "providers") {
		t.Errorf("body = %q, missing providers", string(body))
	}
}

// TestClient_HonorsContextTimeout — ctx-deadline cancels in-flight request.
func TestClient_HonorsContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hang for longer than the ctx deadline.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second) // client timeout high; ctx will win
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := c.GetHealth(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected ctx-cancel error, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("ctx cancellation took %v, want <500ms", elapsed)
	}
}

// TestClient_BaseURLOverridable — caller controls the base URL entirely.
func TestClient_BaseURLOverridable(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true,"data":{"providers":{}}}`)
	}))
	defer srv.Close()

	// Any URL the caller provides is accepted verbatim.
	c := NewClient(srv.URL, time.Second)
	_, _, err := c.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("test server was never called — baseURL was not used")
	}

	// Default base URL via empty string is NOT supplied here — the client is
	// expected to use whatever was passed. The production default lives in the
	// config layer.
	if c2 := NewClient("http://scraper:8088", 0); c2 == nil {
		t.Fatal("NewClient returned nil for zero-timeout case")
	}
}
