package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// fakeProvider matches the one in service/orchestrator_test.go (we can't
// import it because *_test.go is unexported). Keep this minimal — handler
// tests only need HealthCheck and Name to be sensible.
type fakeProvider struct {
	name   string
	health domain.Health
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) FindID(context.Context, domain.AnimeRef) (string, error) {
	return "", nil
}
func (f *fakeProvider) ListEpisodes(context.Context, string) ([]domain.Episode, error) {
	return nil, nil
}
func (f *fakeProvider) ListServers(context.Context, string, string) ([]domain.Server, error) {
	return nil, nil
}
func (f *fakeProvider) GetStream(context.Context, string, string, string, domain.Category) (*domain.Stream, error) {
	return nil, nil
}
func (f *fakeProvider) HealthCheck(context.Context) domain.Health { return f.health }

func newTestHandler(t *testing.T, providers ...domain.Provider) *ScraperHandler {
	t.Helper()
	log := logger.Default()
	o := service.NewOrchestrator(log, domain.NewRegistry())
	for _, p := range providers {
		o.Register(p)
	}
	return NewScraperHandler(o, log)
}

// requireJSON asserts the response body is a JSON object containing every
// key=>value match. Returns the decoded map for further assertions.
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

// TestScraperHandler_GetEpisodes_Returns503 verifies the not-yet-implemented
// stub contract.
func TestScraperHandler_GetEpisodes_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1234", nil)
	h.GetEpisodes(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want 503", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["error"] != "not-yet-implemented" {
		t.Errorf("body.error = %v; want %q", body["error"], "not-yet-implemented")
	}
	// JSON numbers decode to float64.
	if phase, ok := body["phase"].(float64); !ok || phase != 15 {
		t.Errorf("body.phase = %v; want 15", body["phase"])
	}
}

func TestScraperHandler_GetServers_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/servers?mal_id=1&episode=1", nil)
	h.GetServers(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want 503", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["error"] != "not-yet-implemented" {
		t.Errorf("body.error = %v; want %q", body["error"], "not-yet-implemented")
	}
}

func TestScraperHandler_GetStream_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/stream?mal_id=1&episode=1&server=megacloud", nil)
	h.GetStream(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want 503", resp.StatusCode)
	}
	body := requireJSON(t, resp)
	if body["error"] != "not-yet-implemented" {
		t.Errorf("body.error = %v; want %q", body["error"], "not-yet-implemented")
	}
}

// TestScraperHandler_GetHealth_Returns200WithSnapshot verifies the zero-provider
// path: 200 + non-nil empty providers map.
func TestScraperHandler_GetHealth_Returns200WithSnapshot(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health", nil)
	h.GetHealth(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}

	// Decode into the wrapping httputil.Response shape: {success, data:{providers:{...}}}.
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
	if len(wrapper.Data.Providers) != 0 {
		t.Errorf("providers len = %d; want 0", len(wrapper.Data.Providers))
	}
}

// TestScraperHandler_GetHealth_ReflectsRegisteredProvider verifies that a
// registered fake provider's HealthCheck output round-trips through the JSON.
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
		t.Fatalf("missing provider key %q; got %+v", "fakeprov", wrapper.Data.Providers)
	}
	if gotHealth.Provider != "fakeprov" {
		t.Errorf("provider.Provider = %q; want fakeprov", gotHealth.Provider)
	}
	if !gotHealth.Stages["find_id"].Up {
		t.Errorf("find_id.Up = false; want true")
	}
	if gotHealth.Stages["list_episodes"].Up {
		t.Errorf("list_episodes.Up = true; want false")
	}
	if gotHealth.Stages["list_episodes"].LastErr != "broken" {
		t.Errorf("list_episodes.LastErr = %q; want broken", gotHealth.Stages["list_episodes"].LastErr)
	}
}
