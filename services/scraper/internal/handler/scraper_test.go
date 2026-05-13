package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// fakeProvider is a programmable domain.Provider used to drive the handler
// tests without standing up a real upstream. Each method returns a value or
// an error configured by the test; the FindID identity-shim returns the
// AnimeRef's ShikimoriID so the handler's MAL-ID → providerID resolution
// path is a no-op for tests that don't care.
type fakeProvider struct {
	name string

	findIDResult string
	findIDErr    error

	listEpisodesResult []domain.Episode
	listEpisodesErr    error

	listServersResult []domain.Server
	listServersErr    error

	getStreamResult *domain.Stream
	getStreamErr    error

	health domain.Health
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) FindID(_ context.Context, ref domain.AnimeRef) (string, error) {
	if f.findIDErr != nil {
		return "", f.findIDErr
	}
	if f.findIDResult != "" {
		return f.findIDResult, nil
	}
	// Identity-shim: tests that don't set findIDResult get ShikimoriID echoed
	// back so the orchestrator hop to ListEpisodes is direct.
	return ref.ShikimoriID, nil
}

func (f *fakeProvider) ListEpisodes(context.Context, string) ([]domain.Episode, error) {
	return f.listEpisodesResult, f.listEpisodesErr
}

func (f *fakeProvider) ListServers(context.Context, string, string) ([]domain.Server, error) {
	return f.listServersResult, f.listServersErr
}

func (f *fakeProvider) GetStream(context.Context, string, string, string, domain.Category) (*domain.Stream, error) {
	return f.getStreamResult, f.getStreamErr
}

func (f *fakeProvider) HealthCheck(context.Context) domain.Health { return f.health }

func newTestHandler(t *testing.T, providers ...domain.Provider) *ScraperHandler {
	t.Helper()
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	for _, p := range providers {
		o.Register(p)
	}
	return NewScraperHandler(o, nil, log)
}

// newTestHandlerWithCache constructs a handler with both an orchestrator
// (zero registered providers by default) and a real InMemoryHealthCache so
// the admin endpoint tests can drive both surfaces.
func newTestHandlerWithCache(t *testing.T, cache *health.InMemoryHealthCache, providers ...domain.Provider) *ScraperHandler {
	t.Helper()
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	for _, p := range providers {
		o.Register(p)
	}
	return NewScraperHandler(o, cache, log)
}

// requireJSON asserts the response body is a JSON object and returns the
// decoded map for further assertions.
func requireJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

// metaTried extracts data.meta.tried (success) or top-level meta.tried (error)
// from a decoded response body. Returns nil if not present.
func metaTried(t *testing.T, body map[string]any) []string {
	t.Helper()
	pickFromMap := func(m map[string]any) []string {
		meta, ok := m["meta"].(map[string]any)
		if !ok {
			return nil
		}
		raw, ok := meta["tried"].([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if data, ok := body["data"].(map[string]any); ok {
		if got := pickFromMap(data); got != nil {
			return got
		}
	}
	return pickFromMap(body)
}

// TestScraperHandler_GetEpisodes_Live — a registered fakeProvider returning
// a non-empty episode list yields 200 with the list under data.episodes plus
// data.meta.tried = ["fakeprov"].
func TestScraperHandler_GetEpisodes_Live(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:               "fakeprov",
		listEpisodesResult: []domain.Episode{{ID: "ep1", Number: 1, Title: "Pilot"}},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1234", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["success"] != true {
		t.Errorf("success = %v; want true", body["success"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = nil or not object: %v", body["data"])
	}
	eps, ok := data["episodes"].([]any)
	if !ok || len(eps) != 1 {
		t.Fatalf("episodes = %v; want 1-element array", data["episodes"])
	}
	tried := metaTried(t, body)
	if len(tried) != 1 || tried[0] != "fakeprov" {
		t.Errorf("meta.tried = %v; want [fakeprov]", tried)
	}
}

// TestScraperHandler_GetServers_Live — happy path for /scraper/servers.
func TestScraperHandler_GetServers_Live(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:              "fakeprov",
		listServersResult: []domain.Server{{ID: "srv1", Name: "kwik", Type: domain.CategorySub}},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/servers?mal_id=1&episode=ep1", nil)
	h.GetServers(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	srvs, ok := data["servers"].([]any)
	if !ok || len(srvs) != 1 {
		t.Fatalf("servers = %v; want 1-element", data["servers"])
	}
	tried := metaTried(t, body)
	if len(tried) != 1 || tried[0] != "fakeprov" {
		t.Errorf("meta.tried = %v; want [fakeprov]", tried)
	}
}

// TestScraperHandler_GetStream_Live — happy path for /scraper/stream.
func TestScraperHandler_GetStream_Live(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name: "fakeprov",
		getStreamResult: &domain.Stream{
			Sources: []domain.Source{{URL: "https://kwik.cx/x.m3u8", Type: "hls"}},
		},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/scraper/stream?mal_id=1&episode=ep1&server=srv1&category=sub", nil)
	h.GetStream(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	stream, ok := data["stream"].(map[string]any)
	if !ok {
		t.Fatalf("stream missing: %v", data)
	}
	srcs, ok := stream["sources"].([]any)
	if !ok || len(srcs) != 1 {
		t.Errorf("sources = %v; want 1-element", stream["sources"])
	}
	tried := metaTried(t, body)
	if len(tried) != 1 || tried[0] != "fakeprov" {
		t.Errorf("meta.tried = %v; want [fakeprov]", tried)
	}
}

// TestScraperHandler_GetEpisodes_NotFound — orchestrator returns ErrNotFound
// → 404 with meta.tried still present.
func TestScraperHandler_GetEpisodes_NotFound(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:            "fakeprov",
		listEpisodesErr: domain.WrapNotFound(errors.New("no episodes"), "fake: not found"),
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["success"] != false {
		t.Errorf("success = %v; want false", body["success"])
	}
	tried := metaTried(t, body)
	if len(tried) != 1 || tried[0] != "fakeprov" {
		t.Errorf("meta.tried = %v; want [fakeprov] (SCRAPER-NF-05)", tried)
	}
}

// TestScraperHandler_GetEpisodes_ProviderDown — ErrProviderDown → 502.
func TestScraperHandler_GetEpisodes_ProviderDown(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:            "fakeprov",
		listEpisodesErr: domain.WrapProviderDown(errors.New("upstream timeout"), "fake: down"),
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d; want 502", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	tried := metaTried(t, body)
	if len(tried) != 1 || tried[0] != "fakeprov" {
		t.Errorf("meta.tried = %v; want [fakeprov]", tried)
	}
}

// TestScraperHandler_GetEpisodes_NoProviders — zero providers registered →
// 503 NO_PROVIDERS with meta.tried = [].
func TestScraperHandler_GetEpisodes_NoProviders(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t) // zero providers

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want 503", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["success"] != false {
		t.Errorf("success = %v; want false", body["success"])
	}
	tried := metaTried(t, body)
	if tried == nil {
		t.Errorf("meta.tried missing entirely; want present (possibly empty array)")
	}
	if len(tried) != 0 {
		t.Errorf("meta.tried = %v; want empty []", tried)
	}
}

// fakeGatedHandlerProvider satisfies both domain.Provider and the
// service.gatedProvider optional interface. Used by Plan 21-03 Task 4
// regression tests that verify data.meta.gated propagates end-to-end
// when the orchestrator routes a request through a gated-capable
// provider.
type fakeGatedHandlerProvider struct {
	fakeProvider
	gatedStream *domain.Stream
	gatedFlag   bool
	gatedErr    error
}

func (f *fakeGatedHandlerProvider) ListServers(_ context.Context, _, _ string) ([]domain.Server, error) {
	if f.listServersErr != nil {
		return nil, f.listServersErr
	}
	if len(f.listServersResult) > 0 {
		return f.listServersResult, nil
	}
	return []domain.Server{{ID: "https://otakuhg.site/e/x"}}, nil
}

func (f *fakeGatedHandlerProvider) GetStreamWithGate(
	_ context.Context,
	_, _, _ string,
	_ domain.Category,
	_ []domain.Server,
) (*domain.Stream, bool, error) {
	return f.gatedStream, f.gatedFlag, f.gatedErr
}

// TestGetStream_MetaGatedAbsentByDefault — Phase 21 / SCRAPER-HEAL-07
// regression: a /scraper/stream success response from a NON-gated
// provider (animepahe-shape — does not implement GetStreamWithGate)
// MUST emit data.meta.tried but MUST NOT include the data.meta.gated
// key. The FE (Plan 21-04) treats missing-or-false meta.gated as
// "skip Phase 3 of the loader".
//
// After Plan 21-03 Task 4 the handler calls orchestrator.GetStreamGated,
// which falls back to plain GetStream + gated=false on providers that
// don't satisfy the gatedProvider optional interface.
func TestGetStream_MetaGatedAbsentByDefault(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name: "fakeprov",
		getStreamResult: &domain.Stream{
			Sources: []domain.Source{{URL: "https://kwik.cx/x.m3u8", Type: "hls"}},
		},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/scraper/stream?mal_id=1&episode=ep1&server=srv1&category=sub", nil)
	h.GetStream(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	if _, ok := meta["tried"].([]any); !ok {
		t.Errorf("data.meta.tried missing or wrong type; Phase 16 regression: %v", meta)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("data.meta.gated key present on default (gated=false) call; want OMITTED. meta=%v", meta)
	}
}

// TestWriteSuccess_GatedTrueEmitsField — SCRAPER-HEAL-07 direct unit test
// on writeSuccess: when gated=true, the response envelope MUST contain
// data.meta.gated == true AND data.meta.tried as a non-nil array.
func TestWriteSuccess_GatedTrueEmitsField(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	// Drive writeSuccess directly so we don't depend on plumbing the bool
	// through GetStream's call chain (that lands in Plan 21-03).
	h.writeSuccess(rec, map[string]any{"stream": map[string]any{"sources": []any{}}}, []string{"fakeprov"}, true)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	gated, ok := meta["gated"].(bool)
	if !ok {
		t.Fatalf("data.meta.gated missing or wrong type; meta=%v", meta)
	}
	if !gated {
		t.Errorf("data.meta.gated = false; want true")
	}
	if _, ok := meta["tried"].([]any); !ok {
		t.Errorf("data.meta.tried missing; Phase 16 envelope regression: %v", meta)
	}
}

// TestGetStream_MetaGatedTrue_FromGatedProvider — Plan 21-03 Task 4
// end-to-end wiring proof: when the orchestrator routes through a
// gated-capable provider (gogoanime-shape) and the provider returns
// gated=true, the handler MUST emit data.meta.gated=true in the JSON
// envelope. This is the SCRAPER-HEAL-07 cold-path signal the FE uses
// to render Phase 3 of the loader.
func TestGetStream_MetaGatedTrue_FromGatedProvider(t *testing.T) {
	t.Parallel()
	gp := &fakeGatedHandlerProvider{
		fakeProvider: fakeProvider{name: "gogoanime"},
		gatedStream: &domain.Stream{
			Sources: []domain.Source{{URL: "https://otakuhg.site/e/x/master.m3u8", Type: "hls"}},
		},
		gatedFlag: true,
	}
	h := newTestHandler(t, gp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/scraper/stream?mal_id=52991&episode=ep1&server=https://otakuhg.site/e/x&category=sub", nil)
	h.GetStream(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	gated, ok := meta["gated"].(bool)
	if !ok {
		t.Fatalf("data.meta.gated missing or wrong type; meta=%v", meta)
	}
	if !gated {
		t.Errorf("data.meta.gated = false; want true (gated provider returned true)")
	}
	tried, ok := meta["tried"].([]any)
	if !ok || len(tried) != 1 {
		t.Errorf("data.meta.tried missing or wrong length: %v", meta["tried"])
	}
}

// TestGetStream_MetaGatedAbsent_FromGatedProvider_WarmPath — when the
// gated provider returns gated=false (warm-cache hit / caller pin),
// data.meta.gated MUST be absent so cache-hit responses stay
// byte-identical to Phase 16's shape.
func TestGetStream_MetaGatedAbsent_FromGatedProvider_WarmPath(t *testing.T) {
	t.Parallel()
	gp := &fakeGatedHandlerProvider{
		fakeProvider: fakeProvider{name: "gogoanime"},
		gatedStream: &domain.Stream{
			Sources: []domain.Source{{URL: "https://otakuhg.site/e/x/master.m3u8", Type: "hls"}},
		},
		gatedFlag: false, // warm-cache hit — gate did NOT run on this call
	}
	h := newTestHandler(t, gp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/scraper/stream?mal_id=52991&episode=ep1&server=https://otakuhg.site/e/x&category=sub", nil)
	h.GetStream(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("data.meta.gated key present when provider returned gated=false; want OMITTED. meta=%v", meta)
	}
	if _, ok := meta["tried"].([]any); !ok {
		t.Errorf("data.meta.tried missing on warm-path: %v", meta)
	}
}

// TestWriteSuccess_GatedFalseOmitsField — SCRAPER-HEAL-07 direct unit
// test: when gated=false, data.meta.gated MUST be absent (not "gated":
// false) so cache-hit responses stay byte-identical to Phase 16's shape
// and don't churn the FE diffs.
func TestWriteSuccess_GatedFalseOmitsField(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.writeSuccess(rec, map[string]any{"stream": map[string]any{"sources": []any{}}}, []string{"fakeprov"}, false)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("data.meta.gated key present when gated=false; want OMITTED. meta=%v", meta)
	}
	if _, ok := meta["tried"].([]any); !ok {
		t.Errorf("data.meta.tried missing on gated=false: %v", meta)
	}
}

// TestGetEpisodes_NoGatedField — /scraper/episodes is NOT a stream-resolution
// path; its responses MUST NOT include meta.gated regardless of the
// orchestrator state.
func TestGetEpisodes_NoGatedField(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:               "fakeprov",
		listEpisodesResult: []domain.Episode{{ID: "ep1", Number: 1, Title: "Pilot"}},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1234", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("data.meta.gated unexpectedly present on /scraper/episodes: %v", meta)
	}
}

// TestGetServers_NoGatedField — /scraper/servers is NOT a stream-resolution
// path; its responses MUST NOT include meta.gated.
func TestGetServers_NoGatedField(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:              "fakeprov",
		listServersResult: []domain.Server{{ID: "srv1", Name: "kwik", Type: domain.CategorySub}},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/servers?mal_id=1&episode=ep1", nil)
	h.GetServers(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	body := requireJSON(t, resp)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", body)
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		t.Fatalf("data.meta missing: %v", data)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("data.meta.gated unexpectedly present on /scraper/servers: %v", meta)
	}
}

// TestErrorEnvelope_NoGatedField — error responses (NotFound / ProviderDown)
// MUST include meta.tried but MUST NOT include meta.gated.
func TestErrorEnvelope_NoGatedField(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name:            "fakeprov",
		listEpisodesErr: domain.WrapNotFound(errors.New("nope"), "fake: not found"),
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	body := requireJSON(t, resp)
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("top-level meta missing on error envelope: %v", body)
	}
	if _, ok := meta["tried"].([]any); !ok {
		t.Errorf("meta.tried missing on error envelope: %v", meta)
	}
	if _, has := meta["gated"]; has {
		t.Errorf("meta.gated unexpectedly present on error envelope: %v", meta)
	}
}

// TestScraperHandler_GetStream_RespectsPrefer — with two providers
// (first/second registered in that order), prefer=second moves "second"
// to position 0 in meta.tried.
func TestScraperHandler_GetStream_RespectsPrefer(t *testing.T) {
	t.Parallel()
	first := &fakeProvider{
		name: "first",
		getStreamResult: &domain.Stream{
			Sources: []domain.Source{{URL: "https://first.example/x.m3u8", Type: "hls"}},
		},
	}
	second := &fakeProvider{
		name: "second",
		getStreamResult: &domain.Stream{
			Sources: []domain.Source{{URL: "https://second.example/x.m3u8", Type: "hls"}},
		},
	}
	h := newTestHandler(t, first, second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/scraper/stream?mal_id=1&episode=ep1&server=srv1&category=sub&prefer=second", nil)
	h.GetStream(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	tried := metaTried(t, body)
	if len(tried) < 1 || tried[0] != "second" {
		t.Errorf("meta.tried[0] = %v; want \"second\"", tried)
	}
}

// TestScraperHandler_GetEpisodes_MissingMalID — empty mal_id → 400.
func TestScraperHandler_GetEpisodes_MissingMalID(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{name: "fakeprov"}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes", nil)
	h.GetEpisodes(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	tried := metaTried(t, body)
	if tried == nil {
		t.Errorf("meta.tried missing; want present even on 400")
	}
}

// TestScraperHandler_GetHealth_StillWorks — the Phase 15 /scraper/health
// contract is preserved (200 + non-nil providers map).
func TestScraperHandler_GetHealth_StillWorks(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health", nil)
	h.GetHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var wrapper struct {
		Success bool `json:"success"`
		Data    struct {
			Providers map[string]domain.Health `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !wrapper.Success {
		t.Errorf("success = false; want true")
	}
	if wrapper.Data.Providers == nil {
		t.Errorf("providers = nil; want non-nil empty map")
	}
}

// TestScraperHandler_GetHealth_ReflectsRegisteredProvider — health surface
// preserved from Phase 15: registered fakeProvider's HealthCheck output
// round-trips through the JSON.
func TestScraperHandler_GetHealth_ReflectsRegisteredProvider(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name: "fakeprov",
		health: domain.Health{
			Provider: "fakeprov",
			Stages: map[string]domain.StageHealth{
				"find_id":       {Up: true},
				"list_episodes": {Up: false, LastErr: "broken"},
			},
		},
	}
	h := newTestHandler(t, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health", nil)
	h.GetHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var wrapper struct {
		Data struct {
			Providers map[string]domain.Health `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	gotHealth, ok := wrapper.Data.Providers["fakeprov"]
	if !ok {
		t.Fatalf("missing provider key fakeprov; got %+v", wrapper.Data.Providers)
	}
	if gotHealth.Provider != "fakeprov" {
		t.Errorf("provider.Provider = %q; want fakeprov", gotHealth.Provider)
	}
	if !gotHealth.Stages["find_id"].Up {
		t.Errorf("find_id.Up = false; want true")
	}
}

// TestOrchestrator_OrderedProviderNames — orchestrator exposes registered
// provider names in failover order; honors prefer; ignores unknown prefer.
func TestOrchestrator_OrderedProviderNames(t *testing.T) {
	t.Parallel()
	a := &fakeProvider{name: "a"}
	b := &fakeProvider{name: "b"}
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	o.Register(a)
	o.Register(b)

	// No prefer — registration order.
	got := o.OrderedProviderNames("")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("OrderedProviderNames(\"\") = %v; want [a b]", got)
	}
	// prefer=b — b moves to position 0.
	got = o.OrderedProviderNames("b")
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Errorf("OrderedProviderNames(\"b\") = %v; want [b a]", got)
	}
	// prefer=unknown — ignored.
	got = o.OrderedProviderNames("zzz")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("OrderedProviderNames(\"zzz\") = %v; want [a b]", got)
	}
	// Zero providers — empty slice.
	o2 := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	if got := o2.OrderedProviderNames(""); len(got) != 0 {
		t.Errorf("OrderedProviderNames on zero-provider orchestrator = %v; want []", got)
	}
}

// TestParseQuery_PreferLengthCap — WR-01 + REVIEW.md iter-2 WR-NEW-03/04:
// an oversize `prefer` cannot exceed maxPreferLength in the parsed output.
// The regex allow-list (`^[a-z0-9_-]{1,64}$`) structurally enforces the
// cap, so the previous byte-truncation step was removed. Under the new
// implementation an oversized input is rejected (regex no-match → "");
// the contract pinned by this test is simply that the parsed value is
// either empty OR ≤ maxPreferLength — i.e. the upper bound is locked
// regardless of implementation order.
func TestParseQuery_PreferLengthCap(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("a", 1024)
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+huge, nil)
	qp := parseQuery(req)
	if qp.prefer != "" && len(qp.prefer) > maxPreferLength {
		t.Errorf("len(prefer) = %d; want empty or ≤ %d", len(qp.prefer), maxPreferLength)
	}
}

// TestParseQuery_PreferRejectsOversize — REVIEW.md iter-2 WR-NEW-04
// regression. A 65-char prefer value (one byte over the 64-char cap)
// MUST be rejected to "" — proves the regex's `{1,64}` quantifier is
// the active length enforcer, not silent byte truncation. Locks the
// contract that the upper bound is the regex's responsibility now that
// WR-NEW-03 removed the truncation step.
func TestParseQuery_PreferRejectsOversize(t *testing.T) {
	t.Parallel()
	// 65 lowercase chars — every byte individually passes the char-set
	// check (all [a-z]) but the total exceeds the regex's {1,64} cap.
	oversize := strings.Repeat("a", 65)
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+oversize, nil)
	qp := parseQuery(req)
	if qp.prefer != "" {
		t.Errorf("65-char prefer = %q (len=%d); want \"\" (rejected by regex {1,64} cap)",
			qp.prefer, len(qp.prefer))
	}
}

// TestParseQuery_PreferAcceptsBoundary — companion to
// TestParseQuery_PreferRejectsOversize: a 64-char prefer (exactly the
// cap) MUST pass through. Proves the regex is `{1,64}` inclusive, not
// `{1,63}`.
func TestParseQuery_PreferAcceptsBoundary(t *testing.T) {
	t.Parallel()
	boundary := strings.Repeat("a", maxPreferLength)
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+boundary, nil)
	qp := parseQuery(req)
	if qp.prefer != boundary {
		t.Errorf("64-char prefer = %q; want %q (regex {1,64} should accept the boundary)", qp.prefer, boundary)
	}
}

// TestParseQuery_PreferRejectsInvalidChars — WR-09: a `prefer` value that
// contains characters outside [a-z0-9_-] is silently coerced to empty,
// matching the existing "unknown prefer silently ignored" contract. Closes
// the log-injection vector where prefer="animepahe\n[FORGED_LOG]" would
// otherwise reach a structured log field.
func TestParseQuery_PreferRejectsInvalidChars(t *testing.T) {
	t.Parallel()
	// Encoded forms so httptest.NewRequest accepts the URL — the decoded
	// value is what reaches parseQuery via r.URL.Query().Get.
	cases := []struct {
		name string
		// Use URL-encoded forms in the path; decoded value reaches parseQuery.
		raw string
	}{
		{"newline_injection", "animepahe%0a[FORGED]"},
		{"uppercase_only", "ANIMEPAHE"},
		{"path_traversal", "..%2Fetc%2Fpasswd"},
		{"control_null", "anime%00pahe"},
		{"space", "animepahe%20pahe"},
		{"dot_separator", "anime.pahe"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+tc.raw, nil)
			qp := parseQuery(req)
			if qp.prefer != "" {
				t.Errorf("parseQuery(prefer=%q).prefer = %q; want \"\" (silently rejected)", tc.raw, qp.prefer)
			}
		})
	}
}

// TestParseQuery_PreferAcceptsValid — WR-09 sanity check: legitimate
// provider-name shapes pass through unchanged.
func TestParseQuery_PreferAcceptsValid(t *testing.T) {
	t.Parallel()
	cases := []string{"animepahe", "hianime", "9anime_alt", "kodik-ru", "abc123"}
	for _, in := range cases {
		req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+in, nil)
		qp := parseQuery(req)
		if qp.prefer != in {
			t.Errorf("parseQuery(prefer=%q).prefer = %q; want %q", in, qp.prefer, in)
		}
	}
}

// adminResponseEnvelope is the typed wrapper for httputil.OK's envelope so
// the admin endpoint tests can assert structure without typo-prone map keys.
type adminResponseEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Providers   map[string]domain.Health         `json:"providers"`
		Admin       map[string]health.ProviderHealth `json:"admin"`
		GeneratedAt string                           `json:"generated_at"`
	} `json:"data"`
}

func decodeAdminResponse(t *testing.T, resp *http.Response) adminResponseEnvelope {
	t.Helper()
	var env adminResponseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode admin body: %v", err)
	}
	return env
}

// TestAdminHealthHandler_ReturnsCacheSnapshot — cache pre-populated with one
// provider/stage → GET /scraper/health/admin surfaces it under
// data.admin.<provider>.stages.<stage>.{up,last_ok,last_err}.
func TestAdminHealthHandler_ReturnsCacheSnapshot(t *testing.T) {
	t.Parallel()
	cache := health.NewInMemoryHealthCache()
	now := time.Now().UTC().Truncate(time.Second)
	cache.Update("animepahe", health.ProviderHealth{
		Stages: map[string]health.StageStatus{
			health.StageStreamSegment: {Up: true, LastOK: now, LastErr: ""},
		},
		LastUpdated: now,
	})
	h := newTestHandlerWithCache(t, cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
	h.GetAdminHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	env := decodeAdminResponse(t, resp)
	if !env.Success {
		t.Errorf("success = false; want true")
	}
	prov, ok := env.Data.Admin["animepahe"]
	if !ok {
		t.Fatalf("data.admin missing animepahe key; got %+v", env.Data.Admin)
	}
	stage, ok := prov.Stages[health.StageStreamSegment]
	if !ok {
		t.Fatalf("missing stream_segment stage; got %+v", prov.Stages)
	}
	if !stage.Up {
		t.Errorf("stage.Up = false; want true")
	}
	if stage.LastOK.IsZero() {
		t.Errorf("stage.LastOK is zero; want non-zero")
	}
}

// TestAdminHealthHandler_EmptyCacheReturnsEmptyAdmin — empty cache must NOT
// crash and must return a 200 with admin as an empty (but present) object.
func TestAdminHealthHandler_EmptyCacheReturnsEmptyAdmin(t *testing.T) {
	t.Parallel()
	cache := health.NewInMemoryHealthCache()
	h := newTestHandlerWithCache(t, cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
	h.GetAdminHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	env := decodeAdminResponse(t, resp)
	if env.Data.Admin == nil {
		t.Errorf("data.admin = nil; want empty (but non-nil) map")
	}
	if len(env.Data.Admin) != 0 {
		t.Errorf("len(data.admin) = %d; want 0", len(env.Data.Admin))
	}
}

// TestAdminHealthHandler_TruncatesLastErrTo256Chars — defense-in-depth: even
// if a (hypothetical future) caller bypassed the probe's truncation and
// stuffed a 400-char LastErr into the cache, the admin handler MUST clamp
// the visible LastErr to at most MaxLastErrChars (256).
func TestAdminHealthHandler_TruncatesLastErrTo256Chars(t *testing.T) {
	t.Parallel()
	cache := health.NewInMemoryHealthCache()
	longErr := strings.Repeat("x", 400)
	cache.Update("animepahe", health.ProviderHealth{
		Stages: map[string]health.StageStatus{
			health.StageStreamSegment: {Up: false, LastErr: longErr},
		},
		LastUpdated: time.Now(),
	})
	h := newTestHandlerWithCache(t, cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
	h.GetAdminHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	env := decodeAdminResponse(t, resp)
	stage := env.Data.Admin["animepahe"].Stages[health.StageStreamSegment]
	if got := len(stage.LastErr); got > health.MaxLastErrChars {
		t.Errorf("len(last_err) = %d; want <= %d", got, health.MaxLastErrChars)
	}
}

// TestAdminHealthHandler_IncludesGeneratedAt — the response surface includes
// a `generated_at` RFC3339 string so an operator can spot a frozen response
// (e.g. cached upstream by a buggy proxy) at a glance.
func TestAdminHealthHandler_IncludesGeneratedAt(t *testing.T) {
	t.Parallel()
	cache := health.NewInMemoryHealthCache()
	h := newTestHandlerWithCache(t, cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
	h.GetAdminHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	env := decodeAdminResponse(t, resp)
	if env.Data.GeneratedAt == "" {
		t.Fatalf("data.generated_at missing")
	}
	if _, err := time.Parse(time.RFC3339, env.Data.GeneratedAt); err != nil {
		t.Errorf("data.generated_at = %q; not RFC3339: %v", env.Data.GeneratedAt, err)
	}
}

// TestAdminHealthHandler_IncludesPublicProvidersField — the admin endpoint
// MUST also expose the orchestrator's existing HealthSnapshot under the same
// `providers` key as the public /scraper/health endpoint so a downstream
// dashboard can ingest one response instead of two.
func TestAdminHealthHandler_IncludesPublicProvidersField(t *testing.T) {
	t.Parallel()
	fp := &fakeProvider{
		name: "fakeprov",
		health: domain.Health{
			Provider: "fakeprov",
			Stages: map[string]domain.StageHealth{
				"find_id": {Up: true},
			},
		},
	}
	cache := health.NewInMemoryHealthCache()
	h := newTestHandlerWithCache(t, cache, fp)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
	h.GetAdminHealth(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	env := decodeAdminResponse(t, resp)
	if env.Data.Providers == nil {
		t.Fatalf("data.providers nil; want non-nil HealthSnapshot map")
	}
	got, ok := env.Data.Providers["fakeprov"]
	if !ok {
		t.Fatalf("data.providers missing fakeprov; got keys %v", env.Data.Providers)
	}
	if got.Provider != "fakeprov" {
		t.Errorf("data.providers.fakeprov.provider = %q; want fakeprov", got.Provider)
	}
}
