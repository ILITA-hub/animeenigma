// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 3.
//
// PlayerClient tests use httptest.NewServer as a fake player. The two
// methods cover distinct trust boundaries:
//   - FetchUserRecs → /api/users/recs with JWT forwarded (T-03-05: never log it)
//   - FetchListByStatuses → /internal/users/{id}/list with NO JWT
//
// Test #4 explicitly grep-asserts JWT-not-logged via a zaptest observer.

package client

import (
	"context"
	"encoding/json"
	"errors"
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

func TestPlayerClient_FetchUserRecs_HappyPath_ForwardsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/recs" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer testjwt" {
			t.Errorf("expected Authorization=Bearer testjwt, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// Player wraps via httputil.OK → {success:true, data: RecsEnvelope}.
		_, _ = w.Write([]byte(`{"success":true,"data":{"recs":[{"anime":{"id":"a1"}},{"anime":{"id":"a2"}}],"row_label_key":"recs.upNext","total":2,"cache_hit":false,"generated_at":"2026-05-21T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	got, err := c.FetchUserRecs(context.Background(), "testjwt")
	if err != nil {
		t.Fatalf("FetchUserRecs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 recs, got %d", len(got))
	}
	// Decode embedded anime.id to confirm RawMessage pass-through.
	var anime1 struct{ ID string }
	if err := json.Unmarshal(got[0].Anime, &anime1); err != nil {
		t.Fatalf("decode rec[0].anime: %v", err)
	}
	if anime1.ID != "a1" {
		t.Errorf("expected rec[0].anime.id=a1, got %q", anime1.ID)
	}
}

func TestPlayerClient_FetchUserRecs_AnonNoJWT_OmitsAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected NO Authorization header for anon, got %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"recs":[],"row_label_key":"recs.trending","total":0,"cache_hit":false,"generated_at":"2026-05-21T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	got, err := c.FetchUserRecs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchUserRecs anon: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty recs, got %d items", len(got))
	}
}

func TestPlayerClient_FetchUserRecs_5xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	_, err := c.FetchUserRecs(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Errorf("expected 'status=500' in error, got: %v", err)
	}
}

func TestPlayerClient_FetchUserRecs_NeverLogsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	log, recorded := observingLogger()
	c := NewPlayerClient(srv.URL, srv.Client(), log)
	_, _ = c.FetchUserRecs(context.Background(), "supersecretjwttoken-abc123")

	// Every recorded entry's message + fields combined MUST NOT contain the
	// raw token. Concatenate the full structured-log payload as the test surface.
	for _, e := range recorded.All() {
		full := e.Message
		for _, f := range e.Context {
			full += " " + f.String + " "
			if f.Interface != nil {
				// fmt-style string fallback for non-string fields.
				full += " "
			}
		}
		if strings.Contains(full, "supersecretjwttoken-abc123") {
			t.Fatalf("secret JWT leaked into log line: %q", full)
		}
	}
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

func TestPlayerClient_ContextCancellation_Honored(t *testing.T) {
	// Server hangs forever; client ctx is cancelled immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewPlayerClient(srv.URL, srv.Client(), noopLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.FetchUserRecs(ctx, "tok")
	if err == nil {
		t.Fatal("expected ctx-cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled in err chain, got: %v", err)
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
