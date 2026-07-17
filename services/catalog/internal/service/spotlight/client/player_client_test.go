// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 3.
//
// PlayerClient tests use httptest.NewServer as a fake player, covering the
// FetchListByStatuses → /internal/users/{id}/list trust boundary (NO JWT).
//
// FetchUserRecs tests moved to recs_client_test.go on 2026-07-17 (Task 9)
// along with the client itself. noopLogger/observingLogger below are
// shared (same package) with recs_client_test.go.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// noopLogger returns a *logger.Logger whose Sugar swallows everything.
// Use this for tests that don't care about log output.
func noopLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// observingLogger returns (*logger.Logger, *observer.ObservedLogs). All
// log calls land in the observed slice so tests can assert the absence of
// a secret substring (e.g. the JWT value).
func observingLogger() (*logger.Logger, *observer.ObservedLogs) {
	core, recorded := observer.New(zap.DebugLevel)
	z := zap.New(core)
	return &logger.Logger{SugaredLogger: z.Sugar()}, recorded
}

func TestPlayerClient_FetchListByStatuses_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/users/u1/list" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != "watching,planned" {
			t.Errorf("expected status=watching,planned, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("internal endpoint must NOT carry Authorization, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"anime_id":"a1","status":"watching","episodes_aired":12,"episodes_count":12,"last_watched_episode":10,"name":"X","name_ru":"Х","poster_url":"/p.jpg"}]}`))
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	items, err := c.FetchListByStatuses(context.Background(), "u1", []string{"watching", "planned"})
	if err != nil {
		t.Fatalf("FetchListByStatuses: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	got := items[0]
	if got.AnimeID != "a1" || got.Status != "watching" || got.EpisodesAired != 12 ||
		got.EpisodesCount != 12 || got.LastWatchedEpisode != 10 ||
		got.Name != "X" || got.NameRU != "Х" || got.PosterURL != "/p.jpg" {
		t.Errorf("decoded item mismatch: %+v", got)
	}
}

func TestPlayerClient_FetchListByStatuses_EmptyStatuses_NoHTTPCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	items, err := c.FetchListByStatuses(context.Background(), "u1", []string{})
	if err != nil {
		t.Fatalf("expected no error on empty statuses, got %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("expected NO HTTP call for empty statuses, got %d hits", hits)
	}

	// Also: nil statuses.
	items, err = c.FetchListByStatuses(context.Background(), "u1", nil)
	if err != nil || len(items) != 0 {
		t.Errorf("nil statuses: expected ([], nil), got (%v, %v)", items, err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("expected NO HTTP call for nil statuses, got %d hits", hits)
	}
}

func TestPlayerClient_FetchListByStatuses_URLEscapesUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PathEscape on "foo/bar" produces "foo%2Fbar"; chi/router would
		// percent-decode that for a path-param handler. Here we assert the
		// raw URL path (server view) carries the encoded form.
		if r.URL.EscapedPath() != "/internal/users/foo%2Fbar/list" {
			t.Errorf("expected escaped user_id in URL, got %q", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	_, err := c.FetchListByStatuses(context.Background(), "foo/bar", []string{"watching"})
	if err != nil {
		t.Fatalf("FetchListByStatuses with slash-in-id: %v", err)
	}
}

func TestPlayerClient_FetchListByStatuses_5xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	_, err := c.FetchListByStatuses(context.Background(), "u1", []string{"watching"})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Errorf("expected 'status=500' in error, got: %v", err)
	}
}

func TestNewPlayerClient_DefaultsApplied(t *testing.T) {
	c := NewPlayerClient("", nil, noopLogger())
	if c.BaseURL() != "http://player:8083" {
		t.Errorf("default baseURL: want http://player:8083, got %q", c.BaseURL())
	}
	if c.http == nil {
		t.Error("default http client should not be nil")
	}
	if c.http.Timeout != 700*time.Millisecond {
		t.Errorf("default timeout: want 700ms, got %v", c.http.Timeout)
	}
}

func TestNewPlayerClient_OverridesRespected(t *testing.T) {
	custom := &http.Client{Timeout: 100 * time.Millisecond}
	c := NewPlayerClient("http://override:9090", custom, noopLogger())
	if c.BaseURL() != "http://override:9090" {
		t.Errorf("override baseURL: want http://override:9090, got %q", c.BaseURL())
	}
	if c.http != custom {
		t.Error("injected http client should be the exact pointer we passed")
	}
}
