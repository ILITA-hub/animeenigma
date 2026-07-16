package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

// fakeVerifySrc is a stub for the ContentVerifyHandler's verifyProxySource
// dependency. It records every Hint call and returns whatever raw/err are
// configured for RawVerdicts.
type fakeVerifySrc struct {
	mu  sync.Mutex
	raw json.RawMessage
	err error

	hintCalls []hintCall
}

type hintCall struct {
	animeID, visitor, source string
}

func (f *fakeVerifySrc) RawVerdicts(_ context.Context, _ string) (json.RawMessage, error) {
	return f.raw, f.err
}

func (f *fakeVerifySrc) Hint(animeID, visitor, source string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hintCalls = append(f.hintCalls, hintCall{animeID, visitor, source})
}

func (f *fakeVerifySrc) calls() []hintCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]hintCall, len(f.hintCalls))
	copy(out, f.hintCalls)
	return out
}

// fakeCache is a minimal cache.Cache implementing an in-memory JSON store, so
// tests can assert cache-hit/cache-miss handler behavior without Redis. Get/Set
// mirror the real RedisCache's JSON round-trip (libs/cache/cache.go).
type fakeCache struct {
	mu    sync.Mutex
	store map[string][]byte
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string][]byte{}} }

func (f *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	data, ok := f.store[key]
	f.mu.Unlock()
	if !ok {
		return cache.ErrNotFound
	}
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.store[key] = data
	f.mu.Unlock()
	return nil
}

func (f *fakeCache) Delete(_ context.Context, _ ...string) error      { return nil }
func (f *fakeCache) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *fakeCache) Invalidate(_ context.Context, _ string) error     { return nil }
func (f *fakeCache) GetOrSet(_ context.Context, _ string, _ interface{}, _ time.Duration, _ func() (interface{}, error)) error {
	panic("ContentVerifyHandler must not use GetOrSet")
}
func (f *fakeCache) SetNX(_ context.Context, _ string, _ interface{}, _ time.Duration) (bool, error) {
	return false, nil
}

func newContentVerifyRouter(h *handler.ContentVerifyHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/content-verify", h.Get)
	return r
}

// TestContentVerifyHandler_OK asserts a healthy src passes the raw payload
// through verbatim in the standard {"success":true,"data":...} envelope, and
// that the visit hint fired with a non-empty visitor derived from the
// request (scraperUserKey's anonymous IP-hash branch, since httptest.Request
// carries a default RemoteAddr).
func TestContentVerifyHandler_OK(t *testing.T) {
	src := &fakeVerifySrc{raw: json.RawMessage(`{"anime_id":"abc","providers":[{"provider":"gogoanime","summary":{"status":"verified","raw":true,"dub_langs":[],"hardsub_langs":[]},"units":[]}]}`)}
	h := handler.NewContentVerifyHandler(src, nil, nil)
	r := newContentVerifyRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/content-verify", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			AnimeID   string `json:"anime_id"`
			Providers []struct {
				Provider string `json:"provider"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if body.Data.AnimeID != "abc" || len(body.Data.Providers) != 1 || body.Data.Providers[0].Provider != "gogoanime" {
		t.Fatalf("unexpected passthrough body: %+v", body.Data)
	}

	calls := src.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 hint call, got %d: %+v", len(calls), calls)
	}
	if calls[0].animeID != "abc" || calls[0].source != "visit" {
		t.Fatalf("hint call = %+v", calls[0])
	}
	if calls[0].visitor == "" {
		t.Fatal("expected non-empty visitor on the hint call")
	}
}

// TestContentVerifyHandler_SrcError_DegradesToEmpty asserts an upstream
// content-verify failure (network error, non-200, disabled kill switch)
// degrades to a 200 with an empty providers list rather than propagating an
// error — the FE must treat this identically to "nothing verified yet".
func TestContentVerifyHandler_SrcError_DegradesToEmpty(t *testing.T) {
	src := &fakeVerifySrc{err: errors.New("content-verify unreachable")}
	h := handler.NewContentVerifyHandler(src, nil, nil)
	r := newContentVerifyRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/anime/xyz/content-verify", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			AnimeID   string        `json:"anime_id"`
			Providers []interface{} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if body.Data.AnimeID != "xyz" || body.Data.Providers == nil || len(body.Data.Providers) != 0 {
		t.Fatalf("expected empty providers envelope, got %+v (body=%s)", body.Data, rec.Body.String())
	}

	// Even on a degraded-empty response, the visit signal must still have fired.
	if calls := src.calls(); len(calls) != 1 || calls[0].visitor == "" {
		t.Fatalf("expected 1 hint call with a non-empty visitor, got %+v", calls)
	}
}

// TestContentVerifyHandler_MissingAnimeID_400 asserts the animeId path param
// is required.
func TestContentVerifyHandler_MissingAnimeID_400(t *testing.T) {
	src := &fakeVerifySrc{}
	h := handler.NewContentVerifyHandler(src, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/anime//content-verify", nil)
	// Drive Get directly with an empty chi URL param (no router match needed).
	h.Get(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if calls := src.calls(); len(calls) != 0 {
		t.Fatalf("expected no hint call on a bad request, got %+v", calls)
	}
}

// TestContentVerifyHandler_CacheHit_SkipsFetchButStillHints asserts a warm
// cache entry short-circuits the upstream RawVerdicts fetch (the src's raw
// field is left unset, so a fetch would return an empty passthrough instead
// of the cached body) while the visit hint STILL fires — every request is a
// visit signal regardless of cache state (spec §1).
func TestContentVerifyHandler_CacheHit_SkipsFetchButStillHints(t *testing.T) {
	c := newFakeCache()
	cached := json.RawMessage(`{"anime_id":"cached-anime","providers":[]}`)
	if err := c.Set(context.Background(), "contentverify:cached-anime", cached, time.Minute); err != nil {
		t.Fatal(err)
	}
	src := &fakeVerifySrc{err: errors.New("must not be called on a cache hit")}
	h := handler.NewContentVerifyHandler(src, c, nil)
	r := newContentVerifyRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/anime/cached-anime/content-verify", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			AnimeID string `json:"anime_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AnimeID != "cached-anime" {
		t.Fatalf("expected the cached body verbatim, got %+v (body=%s)", body.Data, rec.Body.String())
	}
	if calls := src.calls(); len(calls) != 1 || calls[0].visitor == "" {
		t.Fatalf("expected the hint to still fire on a cache hit, got %+v", calls)
	}
}
