package handler

// Phase 06 (workstream raw-jp / v0.2) tests for the internal cache-
// invalidation endpoint. Verifies path-param validation, idempotent
// behavior on unknown shikimori_id, and that all three raw:* cache
// families for the resolved animeID are deleted while unrelated keys
// survive.
//
// Requires a reachable Redis (defaults to localhost:6379, DB 13 for
// isolation from the resolver tests on DB 14 and SetNX tests on DB
// 15). Skipped on unreachable Redis.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

func newTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 13})
	if err != nil {
		t.Skipf("redis unreachable at %s:%d (%v); skipping internal-cache test", host, port, err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() {
		_ = c.Client().FlushDB(context.Background()).Err()
		_ = c.Close()
	})
	return c
}

// fakeAnimeRepo lets tests inject canned (anime, err) returns from
// GetByShikimoriID. The production *repo.AnimeRepository satisfies
// the same AnimeRepoLike interface.
type fakeAnimeRepo struct {
	byShikimori map[string]*domain.Anime
	err         error
}

func (f *fakeAnimeRepo) GetByShikimoriID(_ context.Context, sid string) (*domain.Anime, error) {
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.byShikimori[sid]
	if !ok {
		return nil, nil // not-found convention
	}
	return a, nil
}

func newTestRouter(t *testing.T, h *InternalCacheHandler) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/internal/cache/invalidate/raw/{shikimoriId}", h.InvalidateRaw)
	return r
}

const testAnimeID = "anime-uuid-AAAA"
const testShikimoriID = "57466"

func TestInternalCache_Happy_DeletesAllThreeFamilies_KeepsOthers(t *testing.T) {
	rc := newTestRedis(t)
	repo := &fakeAnimeRepo{
		byShikimori: map[string]*domain.Anime{
			testShikimoriID: {ID: testAnimeID, ShikimoriID: testShikimoriID},
		},
	}
	h := NewInternalCacheHandler(rc, repo, nil)
	router := newTestRouter(t, h)

	ctx := context.Background()
	// Seed the three families for our animeID + one unrelated key.
	_ = rc.Set(ctx, fmt.Sprintf("%s:%s:1", service.CacheKeySourceDecision, testAnimeID), "library", time.Hour)
	_ = rc.Set(ctx, fmt.Sprintf("%s:%s:2", service.CacheKeySourceDecision, testAnimeID), "allanime", time.Hour)
	_ = rc.Set(ctx, fmt.Sprintf("%s:%s:1:", service.CacheKeyStream, testAnimeID), map[string]string{"foo": "bar"}, time.Hour)
	_ = rc.Set(ctx, fmt.Sprintf("%s:%s", service.CacheKeyEpisodes, testAnimeID), []string{"e1", "e2"}, time.Hour)
	otherKey := fmt.Sprintf("%s:OTHER:1", service.CacheKeySourceDecision)
	_ = rc.Set(ctx, otherKey, "library", time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/internal/cache/invalidate/raw/"+testShikimoriID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	// All three families for testAnimeID must be gone.
	checks := []string{
		fmt.Sprintf("%s:%s:1", service.CacheKeySourceDecision, testAnimeID),
		fmt.Sprintf("%s:%s:2", service.CacheKeySourceDecision, testAnimeID),
		fmt.Sprintf("%s:%s:1:", service.CacheKeyStream, testAnimeID),
		fmt.Sprintf("%s:%s", service.CacheKeyEpisodes, testAnimeID),
	}
	for _, k := range checks {
		exists, err := rc.Exists(ctx, k)
		if err != nil {
			t.Fatalf("EXISTS %s: %v", k, err)
		}
		if exists {
			t.Errorf("key still present after invalidate: %s", k)
		}
	}

	// Unrelated key must survive.
	exists, err := rc.Exists(ctx, otherKey)
	if err != nil {
		t.Fatalf("EXISTS %s: %v", otherKey, err)
	}
	if !exists {
		t.Errorf("unrelated key was deleted: %s", otherKey)
	}
}

func TestInternalCache_UnknownShikimoriID_Idempotent200(t *testing.T) {
	rc := newTestRedis(t)
	repo := &fakeAnimeRepo{byShikimori: map[string]*domain.Anime{}} // empty
	h := NewInternalCacheHandler(rc, repo, nil)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodPost, "/internal/cache/invalidate/raw/9999999", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent), body: %s", rr.Code, rr.Body.String())
	}
}

func TestInternalCache_BadShikimoriIDFormat_400(t *testing.T) {
	rc := newTestRedis(t)
	repo := &fakeAnimeRepo{byShikimori: map[string]*domain.Anime{}}
	h := NewInternalCacheHandler(rc, repo, nil)
	router := newTestRouter(t, h)

	badIDs := []string{
		"id;DROP", // semicolon
		"id.dot",  // dot
		"id$cash", // dollar
	}
	for _, bad := range badIDs {
		t.Run(bad, func(t *testing.T) {
			// Note: chi may URL-decode; we test plain ASCII illegal characters
			// the regex rejects.
			req := httptest.NewRequest(http.MethodPost, "/internal/cache/invalidate/raw/"+bad, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 for %q (body: %s)", rr.Code, bad, rr.Body.String())
			}
		})
	}
}

func TestInternalCache_MethodNotAllowed_405(t *testing.T) {
	rc := newTestRedis(t)
	repo := &fakeAnimeRepo{}
	h := NewInternalCacheHandler(rc, repo, nil)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/internal/cache/invalidate/raw/57466", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestInternalCache_RepoError_500(t *testing.T) {
	rc := newTestRedis(t)
	repo := &fakeAnimeRepo{err: errors.New("database boom")}
	h := NewInternalCacheHandler(rc, repo, nil)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodPost, "/internal/cache/invalidate/raw/57466", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (body: %s)", rr.Code, rr.Body.String())
	}
}
