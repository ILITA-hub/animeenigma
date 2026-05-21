package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fixtureChangelog matches the verified shape of
// frontend/web/public/changelog.json (2026-05-21).
const fixtureChangelog = `[
  {
    "date": "2026-05-21",
    "entries": [
      {"type": "feature", "message": "first"},
      {"type": "feature", "message": "second"}
    ]
  },
  {
    "date": "2026-05-20",
    "entries": [
      {"type": "fix", "message": "third"},
      {"type": "perf", "message": "fourth"}
    ]
  }
]`

func TestWebClient_GetChangelog_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/changelog.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureChangelog))
	}))
	defer srv.Close()

	c := NewWebClient(srv.URL, srv.Client())
	entries, err := c.GetChangelog(context.Background())
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (capped), got %d", len(entries))
	}
	// First two entries inherit "2026-05-21"; third inherits "2026-05-20".
	if entries[0].Date != "2026-05-21" || entries[1].Date != "2026-05-21" {
		t.Errorf("first two entries should have Date=2026-05-21, got %q, %q", entries[0].Date, entries[1].Date)
	}
	if entries[2].Date != "2026-05-20" {
		t.Errorf("third entry should have Date=2026-05-20, got %q", entries[2].Date)
	}
	if entries[0].Message != "first" || entries[1].Message != "second" || entries[2].Message != "third" {
		t.Errorf("messages not preserved in flatten order: %+v", entries)
	}
	if entries[0].Type != "feature" || entries[2].Type != "fix" {
		t.Errorf("types not preserved: %+v", entries)
	}
}

func TestWebClient_GetChangelog_Caps3(t *testing.T) {
	// 5 inner entries across 1 outer group → returned len must be 3.
	body := `[{"date":"2026-05-21","entries":[
    {"type":"feature","message":"a"},
    {"type":"feature","message":"b"},
    {"type":"feature","message":"c"},
    {"type":"feature","message":"d"},
    {"type":"feature","message":"e"}
  ]}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewWebClient(srv.URL, srv.Client())
	entries, err := c.GetChangelog(context.Background())
	if err != nil {
		t.Fatalf("GetChangelog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected len==3, got %d", len(entries))
	}
	// Order preserved: a, b, c.
	if entries[0].Message != "a" || entries[1].Message != "b" || entries[2].Message != "c" {
		t.Errorf("first-3 ordering broken: %+v", entries)
	}
}

func TestWebClient_GetChangelog_HandlesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewWebClient(srv.URL, srv.Client())
	_, err := c.GetChangelog(context.Background())
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("expected status-500 in error message, got: %v", err)
	}
}

func TestWebClient_GetChangelog_ContextCanceled(t *testing.T) {
	// Server delays well beyond client ctx timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			_, _ = w.Write([]byte(`[]`))
		case <-r.Context().Done():
			return
		}
	}))
	defer srv.Close()

	c := NewWebClient(srv.URL, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.GetChangelog(ctx)
	if err == nil {
		t.Fatal("expected ctx-deadline error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded in chain, got: %v", err)
	}
}

func TestWebClient_GetChangelog_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := NewWebClient(srv.URL, srv.Client())
	_, err := c.GetChangelog(context.Background())
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected 'decode' in error, got: %v", err)
	}
}

func TestNewWebClient_Defaults(t *testing.T) {
	c := NewWebClient("", nil)
	if c.BaseURL() != "http://web:80" {
		t.Errorf("default baseURL: want http://web:80, got %q", c.BaseURL())
	}
	if c.http == nil {
		t.Error("default http client should not be nil")
	}
}

func TestNewWebClient_OverridesRespected(t *testing.T) {
	custom := &http.Client{Timeout: 100 * time.Millisecond}
	c := NewWebClient("http://override:8080", custom)
	if c.BaseURL() != "http://override:8080" {
		t.Errorf("override baseURL: want http://override:8080, got %q", c.BaseURL())
	}
	if c.http != custom {
		t.Error("injected http client should be the exact pointer we passed")
	}
}
