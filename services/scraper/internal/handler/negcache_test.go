// Negative-result cache tests (owner directive 2026-07-20): a failed chain
// result is cached for 1h and replayed to identical requests without touching
// any provider; successes are never cached; a nil cache preserves the old
// behavior byte-for-byte (modulo the 502→503 status change tested in
// scraper_test.go).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// negMemCache is the minimal in-memory cache.Cache used by these tests
// (mirrors the memCache fake in service/orchestrator_phase18_test.go).
type negMemCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newNegMemCache() *negMemCache { return &negMemCache{data: map[string][]byte{}} }

func (m *negMemCache) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[key]
	if !ok {
		return errors.New("miss")
	}
	return json.Unmarshal(b, dest)
}

func (m *negMemCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = b
	return nil
}

func (m *negMemCache) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *negMemCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *negMemCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := m.Get(ctx, key, dest); err == nil {
		return nil
	}
	v, err := fn()
	if err != nil {
		return err
	}
	if err := m.Set(ctx, key, v, ttl); err != nil {
		return err
	}
	return m.Get(ctx, key, dest)
}

func (m *negMemCache) Invalidate(context.Context, string) error { return nil }

func (m *negMemCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	_, ok := m.data[key]
	m.mu.Unlock()
	if ok {
		return false, nil
	}
	return true, m.Set(ctx, key, value, ttl)
}

// countingProvider wraps fakeProvider and counts FindID calls — the proof
// that a cache hit never reaches the provider chain.
type countingProvider struct {
	fakeProvider
	mu      sync.Mutex
	findIDs int
}

func (c *countingProvider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	c.mu.Lock()
	c.findIDs++
	c.mu.Unlock()
	return c.fakeProvider.FindID(ctx, ref)
}

func (c *countingProvider) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.findIDs
}

func newNegTestHandler(t *testing.T, p domain.Provider) *ScraperHandler {
	t.Helper()
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	o.Register(p)
	h := NewScraperHandler(o, nil, log)
	h.WithNegCache(NewNegCache(newNegMemCache(), "en", log))
	return h
}

func getEpisodes(t *testing.T, h *ScraperHandler, url string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetEpisodes(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec.Result()
}

// A failing chain is recorded and the second identical request is served
// from the cache: same 503 + code, meta.neg_cached=true, an honest
// error.retry_after_seconds, and — the point of it all — zero additional
// provider work.
func TestNegCache_SecondRequestServedWithoutProvider(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{fakeProvider: fakeProvider{
		name:            "fakeprov",
		listEpisodesErr: domain.WrapProviderDown(errors.New("upstream timeout"), "fake: down"),
	}}
	h := newNegTestHandler(t, cp)

	resp1 := getEpisodes(t, h, "/scraper/episodes?mal_id=1")
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d; want 503", resp1.StatusCode)
	}
	body1 := requireJSON(t, resp1)
	errObj1, _ := body1["error"].(map[string]any)
	if ra, _ := errObj1["retry_after_seconds"].(float64); int(ra) != int(NegTTL.Seconds()) {
		t.Errorf("live retry_after_seconds = %v; want %d", errObj1["retry_after_seconds"], int(NegTTL.Seconds()))
	}
	if calls := cp.calls(); calls != 1 {
		t.Fatalf("FindID calls after first request = %d; want 1", calls)
	}

	resp2 := getEpisodes(t, h, "/scraper/episodes?mal_id=1")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("cached status = %d; want 503", resp2.StatusCode)
	}
	body2 := requireJSON(t, resp2)
	meta, _ := body2["meta"].(map[string]any)
	if nc, _ := meta["neg_cached"].(bool); !nc {
		t.Errorf("meta.neg_cached = %v; want true", meta["neg_cached"])
	}
	errObj2, _ := body2["error"].(map[string]any)
	if code, _ := errObj2["code"].(string); code != codeProviderDown {
		t.Errorf("cached error.code = %q; want %q", code, codeProviderDown)
	}
	if ra, _ := errObj2["retry_after_seconds"].(float64); ra < 1 || ra > NegTTL.Seconds() {
		t.Errorf("cached retry_after_seconds = %v; want within (0, %d]", ra, int(NegTTL.Seconds()))
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Error("cached response missing Retry-After header")
	}
	if calls := cp.calls(); calls != 1 {
		t.Errorf("FindID calls after cached request = %d; want still 1 (cache hit must not touch the chain)", calls)
	}
}

// NotFound outcomes (specific episode / anime unavailable on the chain) are
// cached too, replaying as 404.
func TestNegCache_NotFoundCached(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{fakeProvider: fakeProvider{
		name:      "fakeprov",
		findIDErr: domain.ErrNotFound,
	}}
	h := newNegTestHandler(t, cp)

	for i, wantCalls := range []int{1, 1} {
		resp := getEpisodes(t, h, "/scraper/episodes?mal_id=2")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("request %d status = %d; want 404", i+1, resp.StatusCode)
		}
		resp.Body.Close()
		if calls := cp.calls(); calls != wantCalls {
			t.Fatalf("FindID calls after request %d = %d; want %d", i+1, calls, wantCalls)
		}
	}
}

// Success is never cached: distinct keys don't collide and a healthy chain
// keeps serving live results.
func TestNegCache_SuccessNotCached(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{fakeProvider: fakeProvider{
		name:               "fakeprov",
		listEpisodesResult: []domain.Episode{{ID: "ep1", Number: 1}},
	}}
	h := newNegTestHandler(t, cp)

	for i := 0; i < 2; i++ {
		resp := getEpisodes(t, h, "/scraper/episodes?mal_id=3")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d; want 200", resp.StatusCode)
		}
		resp.Body.Close()
	}
	if calls := cp.calls(); calls != 2 {
		t.Errorf("FindID calls = %d; want 2 (successes are never served from the neg cache)", calls)
	}
}

// The request identity includes the query params — a failure on one episode
// must not shadow a different episode.
func TestNegCache_KeyIncludesParams(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{fakeProvider: fakeProvider{
		name:           "fakeprov",
		listServersErr: domain.WrapProviderDown(errors.New("down"), "fake: down"),
	}}
	h := newNegTestHandler(t, cp)

	do := func(url string) *http.Response {
		rec := httptest.NewRecorder()
		h.GetServers(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec.Result()
	}
	r1 := do("/scraper/servers?mal_id=4&episode=ep1")
	r1.Body.Close()
	r2 := do("/scraper/servers?mal_id=4&episode=ep2")
	r2.Body.Close()
	if calls := cp.calls(); calls != 2 {
		t.Errorf("FindID calls = %d; want 2 (ep1's negative must not shadow ep2)", calls)
	}
}

// An expired entry falls through to the live chain again.
func TestNegCache_ExpiredEntryFallsThrough(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{fakeProvider: fakeProvider{
		name:            "fakeprov",
		listEpisodesErr: domain.WrapProviderDown(errors.New("down"), "fake: down"),
	}}
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	o.Register(cp)
	h := NewScraperHandler(o, nil, log)
	nc := NewNegCache(newNegMemCache(), "en", log)
	h.WithNegCache(nc)

	getEpisodes(t, h, "/scraper/episodes?mal_id=5").Body.Close() // stores at t0

	// Advance the cache's clock past NegTTL — the entry's Until is in the past.
	nc.now = func() time.Time { return time.Now().Add(NegTTL + time.Minute) }
	getEpisodes(t, h, "/scraper/episodes?mal_id=5").Body.Close()
	if calls := cp.calls(); calls != 2 {
		t.Errorf("FindID calls = %d; want 2 (expired entry must re-run the chain)", calls)
	}
}
