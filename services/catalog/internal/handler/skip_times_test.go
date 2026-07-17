package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/go-chi/chi/v5"
)

type skipTimesTestCache struct {
	store       map[string]SkipTimesResult
	setCalls    int
	deleteCalls int
}

func newSkipTimesTestCache() *skipTimesTestCache {
	return &skipTimesTestCache{store: make(map[string]SkipTimesResult)}
}

func (c *skipTimesTestCache) Get(_ context.Context, key string, dest interface{}) error {
	value, ok := c.store[key]
	if !ok {
		return cache.ErrNotFound
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (c *skipTimesTestCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	result, ok := value.(SkipTimesResult)
	if !ok {
		panic("unexpected skip-times cache value type")
	}
	c.setCalls++
	c.store[key] = result
	return nil
}

func (c *skipTimesTestCache) Delete(_ context.Context, keys ...string) error {
	c.deleteCalls++
	for _, key := range keys {
		delete(c.store, key)
	}
	return nil
}
func (c *skipTimesTestCache) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (c *skipTimesTestCache) Invalidate(_ context.Context, _ string) error     { return nil }
func (c *skipTimesTestCache) GetOrSet(_ context.Context, _ string, _ interface{}, _ time.Duration, _ func() (interface{}, error)) error {
	panic("SkipTimesHandler must not use GetOrSet because misses are not cacheable")
}
func (c *skipTimesTestCache) SetNX(_ context.Context, _ string, _ interface{}, _ time.Duration) (bool, error) {
	return false, nil
}

type skipTimesRoundTripper func(*http.Request) (*http.Response, error)

func (f skipTimesRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newSkipTimesTestHandler(c cache.Cache, status int, body string, calls *int) *SkipTimesHandler {
	return &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			*calls++
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
	}
}

func serveSkipTimes(t *testing.T, h *SkipTimesHandler) SkipTimesResult {
	t.Helper()
	router := chi.NewRouter()
	router.Get("/api/skip-times/{malId}/{episode}", h.Get)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/skip-times/59193/3", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data SkipTimesResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope.Data
}

func TestSkipTimesHandlerCachesOnlyUsableTimings(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	h := newSkipTimesTestHandler(c, http.StatusOK, `{
		"found": true,
		"results": [{"interval":{"startTime":90,"endTime":180},"skipType":"op","skipId":"id","episodeLength":1440}]
	}`, &calls)

	first := serveSkipTimes(t, h)
	second := serveSkipTimes(t, h)

	if !first.Found || !second.Found {
		t.Fatalf("expected usable timings, first=%+v second=%+v", first, second)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1 after positive cache hit", calls)
	}
	if c.setCalls != 1 {
		t.Fatalf("cache set calls = %d, want 1", c.setCalls)
	}
}

func TestSkipTimesHandlerDoesNotCacheNotFound(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	h := newSkipTimesTestHandler(c, http.StatusNotFound, `{"found":false,"results":[]}`, &calls)

	first := serveSkipTimes(t, h)
	second := serveSkipTimes(t, h)

	if first.Found || second.Found {
		t.Fatalf("expected not-found responses, first=%+v second=%+v", first, second)
	}
	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2 when misses are not cached", calls)
	}
	if c.setCalls != 0 {
		t.Fatalf("cache set calls = %d, want 0", c.setCalls)
	}
}

func TestSkipTimesHandlerIgnoresLegacyNegativeCacheEntry(t *testing.T) {
	c := newSkipTimesTestCache()
	c.store["skip-times:59193:3"] = SkipTimesResult{Found: false, Results: []SkipTimesResultItem{}}
	calls := 0
	h := newSkipTimesTestHandler(c, http.StatusOK, `{
		"found": true,
		"results": [{"interval":{"startTime":1320,"endTime":1410},"skipType":"ed","skipId":"id","episodeLength":1440}]
	}`, &calls)

	result := serveSkipTimes(t, h)

	if !result.Found || calls != 1 {
		t.Fatalf("legacy miss was not refreshed: result=%+v upstream calls=%d", result, calls)
	}
	if c.setCalls != 1 || !c.store["skip-times:59193:3"].Found {
		t.Fatalf("successful refresh did not replace legacy miss: set calls=%d value=%+v",
			c.setCalls, c.store["skip-times:59193:3"])
	}
	if c.deleteCalls != 1 {
		t.Fatalf("legacy miss delete calls = %d, want 1", c.deleteCalls)
	}
}

func TestHasUsableSkipTimes(t *testing.T) {
	item := func(skipType string, start, end float64) SkipTimesResultItem {
		var result SkipTimesResultItem
		result.SkipType = skipType
		result.Interval.StartTime = start
		result.Interval.EndTime = end
		return result
	}

	tests := []struct {
		name   string
		result SkipTimesResult
		want   bool
	}{
		{name: "not found", result: SkipTimesResult{Found: false}, want: false},
		{name: "empty", result: SkipTimesResult{Found: true}, want: false},
		{name: "zero duration", result: SkipTimesResult{Found: true, Results: []SkipTimesResultItem{item("op", 90, 90)}}, want: false},
		{name: "negative start", result: SkipTimesResult{Found: true, Results: []SkipTimesResultItem{item("ed", -1, 90)}}, want: false},
		{name: "unknown type", result: SkipTimesResult{Found: true, Results: []SkipTimesResultItem{item("recap", 0, 60)}}, want: false},
		{name: "opening", result: SkipTimesResult{Found: true, Results: []SkipTimesResultItem{item("op", 0, 90)}}, want: true},
		{name: "mixed ending", result: SkipTimesResult{Found: true, Results: []SkipTimesResultItem{item("mixed-ed", 1300, 1390)}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasUsableSkipTimes(tt.result); got != tt.want {
				t.Fatalf("hasUsableSkipTimes() = %v, want %v", got, tt.want)
			}
		})
	}
}
