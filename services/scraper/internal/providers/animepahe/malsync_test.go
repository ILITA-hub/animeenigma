package animepahe

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
)

// fakeCache is an in-memory cache.Cache implementation for tests. We don't
// pull in miniredis or the real Redis client here — every test wants
// deterministic behavior and the cache's job is just key/value lookup +
// TTL bookkeeping, both trivial.
type fakeCache struct {
	mu     sync.Mutex
	data   map[string][]byte
	expiry map[string]time.Time
	// hitLog records every Get call for assertions. Order matters.
	getLog []string
	setLog []string
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		data:   make(map[string][]byte),
		expiry: make(map[string]time.Time),
	}
}

func (f *fakeCache) Get(ctx context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLog = append(f.getLog, key)
	data, ok := f.data[key]
	if !ok {
		return cache.ErrNotFound
	}
	if exp, ok := f.expiry[key]; ok && time.Now().After(exp) {
		delete(f.data, key)
		delete(f.expiry, key)
		return cache.ErrNotFound
	}
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setLog = append(f.setLog, key)
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.data[key] = b
	if ttl > 0 {
		f.expiry[key] = time.Now().Add(ttl)
	}
	return nil
}

func (f *fakeCache) Delete(ctx context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.data, k)
		delete(f.expiry, k)
	}
	return nil
}

func (f *fakeCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLog = append(f.getLog, key)
	data, ok := f.data[key]
	if !ok {
		return cache.ErrNotFound
	}
	if exp, ok := f.expiry[key]; ok && time.Now().After(exp) {
		delete(f.data, key)
		delete(f.expiry, key)
		return cache.ErrNotFound
	}
	delete(f.data, key)
	delete(f.expiry, key)
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Exists(ctx context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok, nil
}

func (f *fakeCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := f.Get(ctx, key, dest); err == nil {
		return nil
	}
	v, err := fn()
	if err != nil {
		return err
	}
	if err := f.Set(ctx, key, v, ttl); err != nil {
		return err
	}
	return f.Get(ctx, key, dest)
}

func (f *fakeCache) Invalidate(ctx context.Context, pattern string) error { return nil }
func (f *fakeCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	if _, exists := f.data[key]; exists {
		f.mu.Unlock()
		return false, nil
	}
	f.mu.Unlock()
	return true, f.Set(ctx, key, value, ttl)
}

func (f *fakeCache) snapshotSetLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.setLog))
	copy(cp, f.setLog)
	return cp
}

func (f *fakeCache) snapshotGetLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.getLog))
	copy(cp, f.getLog)
	return cp
}

// TestMalSync_Lookup_Cached: positive cache hit short-circuits the HTTP call.
func TestMalSync_Lookup_Cached(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	// Pre-populate the positive cache for MAL 21 → AnimePahe "1".
	if err := fc.Set(context.Background(), "malsync:21:animepahe", "1", malSyncCacheTTL); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("HTTP must not be called on cache hit; got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "21", "animepahe")
	if err != nil {
		t.Fatalf("Lookup err = %v; want nil", err)
	}
	if !ok || id != "1" {
		t.Errorf("Lookup = (%q,%v); want (\"1\",true)", id, ok)
	}
}

// TestMalSync_Lookup_Live200: real upstream shape, expects positive cache write.
func TestMalSync_Lookup_Live200(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	body := `{"id":21,"title":"One Piece","Sites":{"animepahe":{"1":{"identifier":"1","url":"https://animepahe.ru/anime/1","title":"One Piece"}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mal/anime/21" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "21", "animepahe")
	if err != nil {
		t.Fatalf("Lookup err = %v", err)
	}
	if !ok || id != "1" {
		t.Errorf("Lookup = (%q,%v); want (\"1\",true)", id, ok)
	}
	// Positive cache key written?
	sets := fc.snapshotSetLog()
	found := false
	for _, k := range sets {
		if k == "malsync:21:animepahe" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected positive cache write on malsync:21:animepahe; setLog=%v", sets)
	}
	// Second call should hit cache, NOT upstream.
	id2, ok2, err := m.Lookup(context.Background(), "21", "animepahe")
	if err != nil || !ok2 || id2 != "1" {
		t.Errorf("second Lookup = (%q,%v,%v); want (\"1\",true,nil)", id2, ok2, err)
	}
}

// TestMalSync_Lookup_404: upstream 404 ⇒ negative cache for 24h, returns miss.
func TestMalSync_Lookup_404(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"name":"EntityNotFoundError","code":404}`))
	}))
	defer srv.Close()

	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "999", "animepahe")
	if err != nil {
		t.Fatalf("Lookup err = %v; want nil on confirmed miss", err)
	}
	if ok || id != "" {
		t.Errorf("Lookup = (%q,%v); want (\"\",false)", id, ok)
	}
	sets := fc.snapshotSetLog()
	foundMiss := false
	for _, k := range sets {
		if k == "malsync:999:animepahe:miss" {
			foundMiss = true
		}
	}
	if !foundMiss {
		t.Errorf("expected negative-cache write on miss; setLog=%v", sets)
	}
}

// TestMalSync_Lookup_NetworkError: 5xx ⇒ error, no miss cache.
func TestMalSync_Lookup_NetworkError(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "42", "animepahe")
	if err == nil {
		t.Fatal("expected error on 5xx; got nil")
	}
	if ok || id != "" {
		t.Errorf("Lookup = (%q,%v); want (\"\",false)", id, ok)
	}
	sets := fc.snapshotSetLog()
	for _, k := range sets {
		if k == "malsync:42:animepahe:miss" {
			t.Errorf("must NOT write negative cache on transient 5xx; setLog=%v", sets)
		}
	}
}

// TestMalSync_Lookup_NegativeCacheHonored: pre-populated miss cache skips HTTP.
func TestMalSync_Lookup_NegativeCacheHonored(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	if err := fc.Set(context.Background(), "malsync:777:animepahe:miss", true, malSyncMissTTL); err != nil {
		t.Fatal(err)
	}
	httpCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "777", "animepahe")
	if err != nil {
		t.Fatalf("Lookup err = %v; want nil on cached miss", err)
	}
	if ok || id != "" {
		t.Errorf("Lookup = (%q,%v); want (\"\",false)", id, ok)
	}
	if httpCalls != 0 {
		t.Errorf("HTTP must not be called on cached miss; got %d calls", httpCalls)
	}
}

// TestMalSync_Lookup_MissingMalID: empty mal_id short-circuits without HTTP.
func TestMalSync_Lookup_MissingMalID(t *testing.T) {
	t.Parallel()
	fc := newFakeCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("HTTP must not be called for empty mal_id")
	}))
	defer srv.Close()
	m := NewMalSyncClient(fc, WithMalSyncBaseURL(srv.URL), WithMalSyncHTTPClient(srv.Client()))
	id, ok, err := m.Lookup(context.Background(), "", "animepahe")
	if id != "" || ok || err != nil {
		t.Errorf("Lookup empty malID = (%q,%v,%v); want (\"\",false,nil)", id, ok, err)
	}
}

// Compile-time assertion: fakeCache satisfies cache.Cache.
var _ cache.Cache = (*fakeCache)(nil)

// Sanity guard: cache.ErrNotFound must compare cleanly via errors.Is.
func TestCacheErrNotFoundComparable(t *testing.T) {
	if !errors.Is(cache.ErrNotFound, cache.ErrNotFound) {
		t.Fatal("cache.ErrNotFound must satisfy errors.Is reflexively")
	}
}
