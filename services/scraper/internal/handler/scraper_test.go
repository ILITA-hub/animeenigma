package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
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
	return NewScraperHandler(o, log)
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

// TestParseQuery_PreferLengthCap — WR-01: an oversize `prefer` is truncated
// at parse time so a malicious caller can't balloon log/response payloads
// (the value is echoed into meta.tried + structured log fields).
func TestParseQuery_PreferLengthCap(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("A", 1024)
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?prefer="+huge, nil)
	qp := parseQuery(req)
	if len(qp.prefer) != maxPreferLength {
		t.Errorf("len(prefer) = %d; want %d", len(qp.prefer), maxPreferLength)
	}
}
