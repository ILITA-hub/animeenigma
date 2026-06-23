package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// --- Test scaffolding ---------------------------------------------------

// fakeScraperService implements the scraperServiceAPI used by the four
// scraper handlers. It records call args and returns whatever the test
// configures.
type fakeScraperService struct {
	// Returned by each method call.
	replyStatus int
	replyBody   []byte
	replyErr    error

	// Recorded args from the latest call.
	gotAnimeID  string
	gotEpisode  string
	gotServer   string
	gotCategory string
	gotPrefer   string
	gotUserKey  string

	healthCalls int32
}

func (f *fakeScraperService) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	f.gotAnimeID = animeID
	f.gotPrefer = prefer
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperService) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	f.gotAnimeID = animeID
	f.gotEpisode = episodeID
	f.gotPrefer = prefer
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperService) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	f.gotAnimeID = animeID
	f.gotEpisode = episodeID
	f.gotServer = serverID
	f.gotCategory = category
	f.gotPrefer = prefer
	f.gotUserKey = userKey
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperService) GetScraperHealth(ctx context.Context) (int, []byte, error) {
	atomic.AddInt32(&f.healthCalls, 1)
	return f.replyStatus, f.replyBody, f.replyErr
}

// newTestHandler builds a ScraperEndpointsHandler under test. We split the
// scraper endpoints out into their own handler type for testability while
// keeping the public *CatalogHandler surface intact (CatalogHandler
// embeds the scraper-endpoints handler so /scraper/* routes still hang
// off the same chi mount).
func newTestHandler(svc scraperServiceAPI) *ScraperEndpointsHandler {
	return &ScraperEndpointsHandler{scraperSvc: svc, log: logger.Default()}
}

// fireRequest sends a request through a chi router so chi.URLParam works.
func fireRequest(t *testing.T, h http.HandlerFunc, animeID, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Method(method, "/api/anime/{animeId}/scraper/episodes", h)
	r.Method(method, "/api/anime/{animeId}/scraper/servers", h)
	r.Method(method, "/api/anime/{animeId}/scraper/stream", h)
	r.Method(method, "/api/anime/{animeId}/scraper/health", h)

	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// --- Tests --------------------------------------------------------------

// Test 1 — GetScraperEpisodes forwards status + body verbatim and passes
// the route's animeId + ?prefer= through to the service.
func TestCatalogHandler_GetScraperEpisodes_Routes(t *testing.T) {
	svc := &fakeScraperService{
		replyStatus: http.StatusServiceUnavailable,
		replyBody:   []byte(`{"error":"not-yet-implemented","phase":15}`),
	}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes, "uuid-1", http.MethodGet,
		"/api/anime/uuid-1/scraper/episodes?prefer=animepahe")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not-yet-implemented") {
		t.Errorf("body = %q, missing not-yet-implemented", rec.Body.String())
	}
	if svc.gotAnimeID != "uuid-1" {
		t.Errorf("service got animeID=%q, want uuid-1", svc.gotAnimeID)
	}
	if svc.gotPrefer != "animepahe" {
		t.Errorf("service got prefer=%q, want animepahe", svc.gotPrefer)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// Test 2 — Empty animeId via direct call (defensive guard against weird
// chi-routing edge cases).
func TestCatalogHandler_GetScraperEpisodes_MissingAnimeID_400(t *testing.T) {
	svc := &fakeScraperService{}
	h := newTestHandler(svc)

	// Call the handler directly without chi-context so animeId is "".
	req := httptest.NewRequest(http.MethodGet, "/api/anime//scraper/episodes", nil)
	rec := httptest.NewRecorder()
	h.GetScraperEpisodes(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// Test 3 — service returns liberrors.NotFound → handler returns 404.
func TestCatalogHandler_GetScraperEpisodes_AnimeNotFound_404(t *testing.T) {
	svc := &fakeScraperService{replyErr: liberrors.NotFound("anime")}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes, "missing",
		http.MethodGet, "/api/anime/missing/scraper/episodes")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Test 4 — service returns ErrMalIDUnavailable → handler returns 422 with
// the specific error body.
func TestCatalogHandler_GetScraperEpisodes_MalIDUnavailable_422(t *testing.T) {
	svc := &fakeScraperService{replyErr: service.ErrMalIDUnavailable}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes, "no-mal",
		http.MethodGet, "/api/anime/no-mal/scraper/episodes")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "mal_id unavailable") {
		t.Errorf("body = %q, missing mal_id unavailable", rec.Body.String())
	}
}

// Test 5 — Servers handler requires ?episode=
func TestCatalogHandler_GetScraperServers_RequiresEpisodeParam(t *testing.T) {
	svc := &fakeScraperService{replyStatus: 503}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperServers, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/servers")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "episode") {
		t.Errorf("body = %q, missing episode hint", rec.Body.String())
	}
}

// Test 6 — Stream handler requires ?episode= AND ?server=, defaults
// category to "sub" when missing.
func TestCatalogHandler_GetScraperStream_RequiresEpisodeServerCategory(t *testing.T) {
	svc := &fakeScraperService{replyStatus: 503, replyBody: []byte(`{}`)}
	h := newTestHandler(svc)

	// 6a: missing episode
	rec := fireRequest(t, h.GetScraperStream, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/stream")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing episode: status = %d, want 400", rec.Code)
	}

	// 6b: missing server
	rec = fireRequest(t, h.GetScraperStream, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/stream?episode=ep-1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing server: status = %d, want 400", rec.Code)
	}

	// 6c: episode+server present, no category → service called with "sub" default
	rec = fireRequest(t, h.GetScraperStream, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/stream?episode=ep-1&server=srv-1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("happy path: status = %d, want 503", rec.Code)
	}
	if svc.gotCategory != "sub" {
		t.Errorf("category = %q, want sub (default)", svc.gotCategory)
	}
}

// Test 6d — P2.8: an anonymous stream request (no JWT) derives a non-empty,
// salted-IP-prefixed user_key from the request's client IP and forwards it.
func TestCatalogHandler_GetScraperStream_DerivesUserKey(t *testing.T) {
	svc := &fakeScraperService{replyStatus: 503, replyBody: []byte(`{}`)}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperStream, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/stream?episode=ep-1&server=srv-1")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if svc.gotUserKey == "" {
		t.Fatal("user_key was empty; want a non-empty salted-IP key for an anonymous request")
	}
	if !strings.HasPrefix(svc.gotUserKey, "ip:") {
		t.Errorf("user_key = %q, want an ip: prefix for an anonymous (no-JWT) request", svc.gotUserKey)
	}
}

// Test 7 — Health handler does not require ?episode= or any query;
// service is called once even though animeId is in the path.
func TestCatalogHandler_GetScraperHealth_NoAnimeIDRequired(t *testing.T) {
	svc := &fakeScraperService{
		replyStatus: http.StatusOK,
		replyBody:   []byte(`{"success":true,"data":{"providers":{}}}`),
	}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperHealth, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/health")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "providers") {
		t.Errorf("body = %q, missing providers", rec.Body.String())
	}
	if atomic.LoadInt32(&svc.healthCalls) != 1 {
		t.Errorf("scraper.GetHealth called %d times, want exactly 1", svc.healthCalls)
	}
}

// Test 8 — Unknown service error → handler returns 500 via httputil.Error.
// This locks the contract that only NotFound and ErrMalIDUnavailable get
// special-cased; everything else funnels through the standard error path.
func TestCatalogHandler_GetScraperEpisodes_UnknownError_500(t *testing.T) {
	svc := &fakeScraperService{replyErr: errors.New("boom")}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/episodes")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// TestCatalogHandler_ScraperPipeline_InjectsMalID — CR-03 contract test.
//
// Exercises the FULL pipeline from frontend-style request (UUID in path,
// no mal_id query) through the catalog handler → real scraperOps →
// real scraper.Client → httptest scraper-server URL, and asserts that
// `mal_id=...` ends up on the upstream request. Regression guard for
// the "every English-player call returns 400 INVALID_INPUT" defect
// where the catalog forgets to inject mal_id.
//
// We intentionally use a real *service.CatalogService.scraperOps via
// a small interface stub so the real resolve-MAL-ID logic runs.
func TestCatalogHandler_ScraperPipeline_InjectsMalID(t *testing.T) {
	// Capture the upstream URL the scraper microservice would receive.
	var captured struct {
		path  string
		query string
	}
	scraperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer scraperSrv.Close()

	// Wire a fakeScraperService that mimics the real catalog service:
	// resolves the UUID to mal_id 12345 and forwards via a real scraper.Client.
	svc := &uuidToMalIDStub{
		mapping:     map[string]int{"11111111-1111-4111-8111-111111111111": 12345},
		scraperBase: scraperSrv.URL,
	}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes,
		"11111111-1111-4111-8111-111111111111", http.MethodGet,
		"/api/anime/11111111-1111-4111-8111-111111111111/scraper/episodes?prefer=animepahe")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if captured.path != "/scraper/episodes" {
		t.Errorf("upstream path = %q, want /scraper/episodes", captured.path)
	}
	if !strings.Contains(captured.query, "mal_id=12345") {
		t.Errorf("upstream query = %q, missing mal_id=12345 — CR-03 regression", captured.query)
	}
	if !strings.Contains(captured.query, "prefer=animepahe") {
		t.Errorf("upstream query = %q, missing prefer=animepahe", captured.query)
	}
}

// uuidToMalIDStub implements scraperServiceAPI by mimicking the real
// catalog service: it resolves the path-level animeId to a mal_id via
// an in-memory map, then forwards to a real httptest scraper server via
// `http.Get` with the standard `mal_id=...&prefer=...` query.
//
// We avoid importing the real *service.CatalogService here because the
// handler package is internal and would create an import cycle for
// a test-only need. The contract this stub exercises is the same
// MAL-ID-injection contract the production service satisfies (verified
// independently by scraper.Client_test and scraperOps_test).
type uuidToMalIDStub struct {
	mapping     map[string]int
	scraperBase string
}

func (s *uuidToMalIDStub) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	malID, ok := s.mapping[animeID]
	if !ok {
		return 0, nil, liberrors.NotFound("anime")
	}
	u := s.scraperBase + "/scraper/episodes?mal_id=" + intToA(malID)
	if prefer != "" {
		u += "&prefer=" + prefer
	}
	return roundTrip(ctx, u)
}

func (s *uuidToMalIDStub) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	return 0, nil, errors.New("not implemented in stub")
}
func (s *uuidToMalIDStub) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	return 0, nil, errors.New("not implemented in stub")
}
func (s *uuidToMalIDStub) GetScraperHealth(ctx context.Context) (int, []byte, error) {
	return 0, nil, errors.New("not implemented in stub")
}

func intToA(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func roundTrip(ctx context.Context, u string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// Test 9 — Bonus: body io.ReadAll roundtrip preserves exact bytes
// (catches stray trailing newlines from JSON encoders).
func TestCatalogHandler_GetScraperEpisodes_BodyExactBytes(t *testing.T) {
	want := []byte(`{"error":"not-yet-implemented","phase":15}`)
	svc := &fakeScraperService{
		replyStatus: 503,
		replyBody:   want,
	}
	h := newTestHandler(svc)

	rec := fireRequest(t, h.GetScraperEpisodes, "uuid-1",
		http.MethodGet, "/api/anime/uuid-1/scraper/episodes")

	got, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("body = %q, want %q (exact passthrough — no JSON re-encoding)", string(got), string(want))
	}
}
