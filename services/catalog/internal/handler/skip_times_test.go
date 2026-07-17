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
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
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

// fakeSkipSource is the test double for SkipSource. calls tracks invocation
// count so tests can assert the detected-blend cache actually short-circuits
// repeat content-verify lookups.
type fakeSkipSource struct {
	rows  []capability.SkipTimingRow
	calls int
}

func (f *fakeSkipSource) SkipTimings(_ context.Context, _ string) []capability.SkipTimingRow {
	f.calls++
	return f.rows
}

func newSkipTimesTestLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// serveSkipTimesQuery is serveSkipTimes with an optional raw query string
// appended, so detected-blend tests can exercise the anime/provider/team
// params without disturbing the existing malId/episode-only callers above.
func serveSkipTimesQuery(t *testing.T, h *SkipTimesHandler, query string) SkipTimesResult {
	t.Helper()
	router := chi.NewRouter()
	router.Get("/api/skip-times/{malId}/{episode}", h.Get)
	recorder := httptest.NewRecorder()
	target := "/api/skip-times/59193/3"
	if query != "" {
		target += "?" + query
	}
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
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

// aniskipUpstreamFound is the shared fixture: a decodable "positive" aniskip
// response, used whenever a test asserts the AniSkip path was reached.
const aniskipUpstreamFound = `{
	"found": true,
	"results": [{"interval":{"startTime":90,"endTime":180},"skipType":"op","skipId":"id","episodeLength":1440}]
}`

func TestSkipTimesHandlerBlendsDetectedOpAndEd(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{
			Provider: "gogoanime", Team: "", Episode: 3,
			OpStart: 10, OpEnd: 100, OpStatus: "detected",
			EdStart: 1300, EdEnd: 1390, EdStatus: "detected",
		},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(aniskipUpstreamFound))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	query := "anime=11111111-1111-1111-1111-111111111111&provider=gogoanime&team="
	first := serveSkipTimesQuery(t, h, query)
	second := serveSkipTimesQuery(t, h, query)

	for _, result := range []SkipTimesResult{first, second} {
		if !result.Found || result.Source != "detected" {
			t.Fatalf("result = %+v, want found+source=detected", result)
		}
		if len(result.Results) != 2 {
			t.Fatalf("results = %+v, want 2 items (op+ed)", result.Results)
		}
	}
	byType := map[string]SkipTimesResultItem{}
	for _, item := range first.Results {
		byType[item.SkipType] = item
	}
	if op := byType["op"]; op.Interval.StartTime != 10 || op.Interval.EndTime != 100 {
		t.Fatalf("op item = %+v, want 10-100", op)
	}
	if ed := byType["ed"]; ed.Interval.StartTime != 1300 || ed.Interval.EndTime != 1390 {
		t.Fatalf("ed item = %+v, want 1300-1390", ed)
	}
	if calls != 0 {
		t.Fatalf("aniskip upstream calls = %d, want 0 (detected blend short-circuits)", calls)
	}
	if skip.calls != 1 {
		t.Fatalf("SkipTimings calls = %d, want 1 (second request must hit the detected cache)", skip.calls)
	}
	const wantKey = "skip-times:detected:11111111-1111-1111-1111-111111111111:gogoanime::3"
	if _, ok := c.store[wantKey]; !ok {
		t.Fatalf("expected detected result cached under %q, store keys = %v", wantKey, c.store)
	}
}

// TestSkipTimesHandlerCacheKeyEscapesCollidingProviderTeamSplits guards
// against an unescaped ':'-delimited cache key: provider/team are free-form
// (Kodik fansub team titles can contain ':'), so without escaping,
// (provider="kodik", team="A:B") and (provider="kodik:A", team="B") would
// concatenate to the identical cache key and the second lookup would
// incorrectly be served the first lookup's cached detected result.
func TestSkipTimesHandlerCacheKeyEscapesCollidingProviderTeamSplits(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	// Row matches ONLY the first (provider, team) split below.
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{
			Provider: "kodik", Team: "A:B", Episode: 3,
			OpStart: 20, OpEnd: 200, OpStatus: "detected",
		},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(aniskipUpstreamFound))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	const animeID = "44444444-4444-4444-4444-444444444444"

	first := serveSkipTimesQuery(t, h, "anime="+animeID+"&provider=kodik&team=A%3AB")
	if !first.Found || first.Source != "detected" {
		t.Fatalf("first result = %+v, want found+source=detected", first)
	}
	if calls != 0 {
		t.Fatalf("aniskip upstream calls after first request = %d, want 0", calls)
	}

	// Different (provider, team) split of the same raw ':'-joined string.
	// No row matches "kodik:A"/"B" — this must miss the detected cache
	// (distinct key from the first request) and fall through to AniSkip,
	// not reuse the first request's cached "detected" result.
	second := serveSkipTimesQuery(t, h, "anime="+animeID+"&provider=kodik%3AA&team=B")
	if second.Source != "" {
		t.Fatalf("second result = %+v, want empty source — must not reuse the colliding cached entry", second)
	}
	if !second.Found {
		t.Fatalf("second result = %+v, want the aniskip fixture result", second)
	}
	if calls != 1 {
		t.Fatalf("aniskip upstream calls after second request = %d, want 1 (cache miss, fell through)", calls)
	}
}

func TestSkipTimesHandlerBlendsDetectedOpOnlyWhenEdNoMatch(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{
			Provider: "animepahe", Team: "610", Episode: 3,
			OpStart: 5, OpEnd: 95, OpStatus: "detected",
			EdStatus: "no_match",
		},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"found":false,"results":[]}`))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	result := serveSkipTimesQuery(t, h, "anime=22222222-2222-2222-2222-222222222222&provider=animepahe&team=610")

	if !result.Found || result.Source != "detected" {
		t.Fatalf("result = %+v, want found+source=detected", result)
	}
	if len(result.Results) != 1 || result.Results[0].SkipType != "op" {
		t.Fatalf("results = %+v, want exactly one op item", result.Results)
	}
	if result.Results[0].Interval.StartTime != 5 || result.Results[0].Interval.EndTime != 95 {
		t.Fatalf("op item = %+v, want 5-95", result.Results[0])
	}
	if calls != 0 {
		t.Fatalf("aniskip upstream calls = %d, want 0 (detected op alone still short-circuits)", calls)
	}
}

// TestSkipTimesHandlerFallsBackToEmptyTeamRow covers Finding 4: content-verify
// enumerates animejoy skip units with Team="" (animejoy has no fansub/team
// concept on the cv side), but the frontend sends the user's selected fansub
// name as `team` for animejoy. Without the empty-team fallback in
// matchSkipTimingRow, the exact-match lookup would always miss for animejoy
// and a detected window would never serve. This is safe for kodik because
// kodik rows always carry a non-empty Team, so no kodik row can ever exist
// at Team="" for the fallback to incorrectly match against.
func TestSkipTimesHandlerFallsBackToEmptyTeamRow(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{Provider: "animejoy-sibnet", Team: "", Episode: 3, OpStatus: "detected", OpStart: 30, OpEnd: 120},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(aniskipUpstreamFound))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	const animeID = "55555555-5555-5555-5555-555555555555"
	result := serveSkipTimesQuery(t, h, "anime="+animeID+"&provider=animejoy-sibnet&team=AnimeJoy")

	if !result.Found || result.Source != "detected" {
		t.Fatalf("result = %+v, want found+source=detected (fallback to the empty-team row)", result)
	}
	if len(result.Results) != 1 || result.Results[0].SkipType != "op" {
		t.Fatalf("results = %+v, want exactly one op item", result.Results)
	}
	if result.Results[0].Interval.StartTime != 30 || result.Results[0].Interval.EndTime != 120 {
		t.Fatalf("op item = %+v, want 30-120", result.Results[0])
	}
	if calls != 0 {
		t.Fatalf("aniskip upstream calls = %d, want 0 (empty-team fallback short-circuits)", calls)
	}
	// Cache key stays keyed by the REQUESTED team ("AnimeJoy"), not the
	// matched row's actual (empty) Team.
	const wantKey = "skip-times:detected:55555555-5555-5555-5555-555555555555:animejoy-sibnet:AnimeJoy:3"
	if _, ok := c.store[wantKey]; !ok {
		t.Fatalf("expected detected result cached under %q, store keys = %v", wantKey, c.store)
	}
}

func TestSkipTimesHandlerFallsThroughToAniskipWhenNoDetectedMatch(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	// Row exists but for a different episode than the request (path episode
	// is "3") — must not match, so the handler falls through to AniSkip.
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{Provider: "gogoanime", Team: "", Episode: 99, OpStatus: "detected", OpStart: 1, OpEnd: 2},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(aniskipUpstreamFound))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	result := serveSkipTimesQuery(t, h, "anime=33333333-3333-3333-3333-333333333333&provider=gogoanime&team=")

	if result.Source != "" {
		t.Fatalf("source = %q, want empty (aniskip path, no detected match)", result.Source)
	}
	if !result.Found {
		t.Fatalf("expected aniskip result to be served, got %+v", result)
	}
	if calls != 1 {
		t.Fatalf("aniskip upstream calls = %d, want 1 (fallback path)", calls)
	}
	if skip.calls != 1 {
		t.Fatalf("SkipTimings calls = %d, want 1", skip.calls)
	}
}

func TestSkipTimesHandlerParamsAbsentUsesAniskipUnchanged(t *testing.T) {
	c := newSkipTimesTestCache()
	calls := 0
	var gotURL string
	// skip is non-nil (feature enabled) but the request carries no
	// anime/provider params — the detected blend must never engage, and the
	// upstream aniskip URL must be byte-identical to the pre-existing path.
	skip := &fakeSkipSource{rows: []capability.SkipTimingRow{
		{Provider: "gogoanime", Team: "", Episode: 3, OpStatus: "detected", OpStart: 1, OpEnd: 2},
	}}
	h := &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{Transport: skipTimesRoundTripper(func(req *http.Request) (*http.Response, error) {
			calls++
			gotURL = req.URL.String()
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(aniskipUpstreamFound))}, nil
		})},
		log:  newSkipTimesTestLogger(),
		skip: skip,
	}

	result := serveSkipTimesQuery(t, h, "")

	if result.Source != "" {
		t.Fatalf("source = %q, want empty (pure aniskip path)", result.Source)
	}
	if !result.Found || calls != 1 {
		t.Fatalf("expected aniskip result with 1 upstream call, result=%+v calls=%d", result, calls)
	}
	if skip.calls != 0 {
		t.Fatalf("SkipTimings calls = %d, want 0 (params absent must never query content-verify)", skip.calls)
	}
	const wantSuffix = "/v2/skip-times/59193/3?episodeLength=0&types=op&types=ed"
	if !strings.HasSuffix(gotURL, wantSuffix) {
		t.Fatalf("upstream URL = %q, want suffix %q (unchanged from pre-blend behavior)", gotURL, wantSuffix)
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
