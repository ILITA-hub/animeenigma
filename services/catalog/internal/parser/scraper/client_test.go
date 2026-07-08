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
	status, body, err := c.GetEpisodes(context.Background(), 12345, "Bleach", []string{"Burichi", "BLEACH"}, "animepahe", false)
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
	// ISS-017: alternate title forms are forwarded comma-joined as title_alt.
	if !strings.Contains(capturedQuery, "title_alt=") {
		t.Errorf("query = %q, missing title_alt", capturedQuery)
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
	status, body, err := c.GetEpisodes(context.Background(), 1, "", nil, "", false)
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
	status, body, err := c.GetEpisodes(context.Background(), 1, "", nil, "", false)
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
	_, _, err := c.GetServers(context.Background(), 42, "Bleach", nil, "ep-1", "animepahe", false)
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
	_, _, err := c.GetStream(context.Background(), 7, "Bleach", nil, "ep-2", "srv-1", "sub", "animepahe", false, "")
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

func TestGetStream_SendsUserKeyHeader(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-AE-User")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second)
	_, _, err := c.GetStream(context.Background(), 1, "t", nil, "ep", "srv", "sub", "gogoanime", false, "alice")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if gotUser != "alice" {
		t.Errorf("X-AE-User = %q; want alice", gotUser)
	}
}

func TestGetStream_OmitsUserKeyHeaderWhenEmpty(t *testing.T) {
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Ae-User"] // canonicalized
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second)
	if _, _, err := c.GetStream(context.Background(), 1, "t", nil, "ep", "srv", "sub", "gogoanime", false, ""); err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if present {
		t.Error("X-AE-User header present on anon stream call")
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

// TestNewClient_ZeroTimeoutFallback_ExceedsBrowserProviderBudget guards the
// client-level fallback default against the same regression class as
// config_test.go's Load() check: it must stay above the scraper's 35s
// SCRAPER_BROWSER_PROVIDER_TIMEOUT, or a caller that passes timeout==0
// silently cuts off a browser-engine provider's cold-solve attempt.
func TestNewClient_ZeroTimeoutFallback_ExceedsBrowserProviderBudget(t *testing.T) {
	c := NewClient("http://scraper:8088", 0)
	const browserProviderBudget = 35 * time.Second
	if c.httpClient.Timeout <= browserProviderBudget {
		t.Fatalf("zero-timeout fallback = %s, want > %s (SCRAPER_BROWSER_PROVIDER_TIMEOUT)", c.httpClient.Timeout, browserProviderBudget)
	}
}

func TestClient_GetAnime18Episodes_BuildsURL(t *testing.T) {
	var path, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	if _, _, err := c.GetAnime18Episodes(context.Background(), "57", "Akiba Girls", []string{"Akibakei Kanojo"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/anime18/episodes" {
		t.Fatalf("path = %q, want /anime18/episodes", path)
	}
	// mal_id MUST be present (the scraper handler hard-requires it).
	if !strings.Contains(query, "mal_id=57") {
		t.Errorf("query = %q, missing mal_id=57", query)
	}
	if !strings.Contains(query, "title=Akiba") {
		t.Errorf("query = %q, missing title", query)
	}
}

func TestClient_GetAnime18Stream_BuildsURL(t *testing.T) {
	var path, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"stream":{"sources":[]}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second)
	if _, _, err := c.GetAnime18Stream(context.Background(), "0", "Akiba Girls", nil, "472-akiba-girls-episode-1", "turbovid"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/anime18/stream" {
		t.Fatalf("path = %q, want /anime18/stream", path)
	}
	for _, want := range []string{"mal_id=0", "episode=472-akiba-girls-episode-1", "server=turbovid"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, missing %q", query, want)
		}
	}
}

// TestClient_GetEpisodes_ExclusiveParam verifies that GetEpisodes with
// exclusive=true appends exclusive=true to the outbound query string.
func TestClient_GetEpisodes_ExclusiveParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, time.Second)
	_, _, err := c.GetEpisodes(context.Background(), 1, "Frieren", nil, "gogoanime", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "exclusive=true") {
		t.Fatalf("outbound query %q missing exclusive=true", gotQuery)
	}
}
