package handler

// Watch-Together workstream / Phase 04 — WT-STATE-02.
//
// HTTP-level coverage for InternalEpisodesValidateHandler. Drives the
// real chi router so URL-param extraction + the shikimoriIDPattern
// gate are exercised; injects a fake EpisodesValidator (no real
// service / lookup / repo / parser construction needed).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// fakeValidator implements EpisodesValidator via injected fields.
// Captures the args of the last call so tests can assert query-param
// parsing wiring.
type fakeValidator struct {
	result service.ValidateResult
	err    error

	// last call args
	gotShikimori   string
	gotPlayer      string
	gotEpisode     string
	gotTranslation string
	gotWatchType   string
	calls          int
}

func (f *fakeValidator) ValidateEpisode(
	_ context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (service.ValidateResult, error) {
	f.gotShikimori = shikimoriID
	f.gotPlayer = player
	f.gotEpisode = episodeID
	f.gotTranslation = translationID
	f.gotWatchType = watchType
	f.calls++
	return f.result, f.err
}

func newValidateRouter(h *InternalEpisodesValidateHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/internal/anime/{shikimoriId}/episodes/validate", h.Validate)
	return r
}

// validateBody decodes the JSON response envelope's data block into
// ValidateResult. Returns (parsed, success, error-code-if-any).
type apiEnvelope struct {
	Success bool                   `json:"success"`
	Data    service.ValidateResult `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func decodeEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var e apiEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, string(body))
	}
	return e
}

// -----------------------------------------------------------------
// Happy paths
// -----------------------------------------------------------------

func TestValidateHandler_Kodik_Valid_200(t *testing.T) {
	fv := &fakeValidator{result: service.ValidateResult{Valid: true}}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=kodik&episode_id=5&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	env := decodeEnvelope(t, rr.Body.Bytes())
	if !env.Success || !env.Data.Valid || env.Data.Reason != "" {
		t.Fatalf("envelope = %+v, want success+valid+empty-reason", env)
	}
	// Argument wiring
	if fv.gotShikimori != "57466" || fv.gotPlayer != "kodik" ||
		fv.gotEpisode != "5" || fv.gotTranslation != "42" || fv.gotWatchType != "" {
		t.Fatalf("captured args mismatch: %+v", fv)
	}
}

func TestValidateHandler_AnimeLib_WatchTypePassed_200(t *testing.T) {
	fv := &fakeValidator{result: service.ValidateResult{Valid: true}}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=animelib&episode_id=2&translation_id=100&watch_type=sub", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if fv.gotWatchType != "sub" {
		t.Fatalf("watch_type not propagated, got %q", fv.gotWatchType)
	}
}

func TestValidateHandler_OurEnglish_NoTranslationRequired_200(t *testing.T) {
	fv := &fakeValidator{result: service.ValidateResult{Valid: true}}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=ourenglish&episode_id=1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	env := decodeEnvelope(t, rr.Body.Bytes())
	if !env.Data.Valid {
		t.Fatalf("ourenglish permissive: want valid=true, got %+v", env.Data)
	}
}

// -----------------------------------------------------------------
// Soft-negative (200 with valid=false)
// -----------------------------------------------------------------

func TestValidateHandler_EpisodeUnavailable_200(t *testing.T) {
	fv := &fakeValidator{result: service.ValidateResult{
		Valid: false, Reason: service.ReasonEpisodeUnavailable,
	}}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=kodik&episode_id=9999&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	env := decodeEnvelope(t, rr.Body.Bytes())
	if env.Data.Valid || env.Data.Reason != service.ReasonEpisodeUnavailable {
		t.Fatalf("envelope = %+v, want valid=false reason=%q",
			env.Data, service.ReasonEpisodeUnavailable)
	}
}

func TestValidateHandler_EmptyEpisode_Permissive_Still200(t *testing.T) {
	// Handler does NOT 400 on empty episode_id — service decides.
	fv := &fakeValidator{result: service.ValidateResult{
		Valid: false, Reason: service.ReasonEpisodeUnavailable,
	}}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=raw&episode_id=", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// -----------------------------------------------------------------
// Hard input errors — 400
// -----------------------------------------------------------------

func TestValidateHandler_MissingPlayer_400(t *testing.T) {
	fv := &fakeValidator{}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?episode_id=1&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if fv.calls != 0 {
		t.Fatalf("service must not be called on missing player; calls=%d", fv.calls)
	}
}

func TestValidateHandler_UnknownPlayer_400(t *testing.T) {
	fv := &fakeValidator{}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=bogus&episode_id=1&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "player not supported") {
		t.Fatalf("expected 'player not supported' message, got %s", rr.Body.String())
	}
	if fv.calls != 0 {
		t.Fatalf("service must not be called on bogus player; calls=%d", fv.calls)
	}
}

func TestValidateHandler_BadShikimoriIDPattern_400(t *testing.T) {
	fv := &fakeValidator{}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	// "../" trips path-traversal guard; chi's URLParam returns
	// the literal segment.
	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/bad%20id/episodes/validate?player=kodik&episode_id=1&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if fv.calls != 0 {
		t.Fatalf("service must not be called on bad shikimori_id; calls=%d", fv.calls)
	}
}

// -----------------------------------------------------------------
// Service-error → status mapping
// -----------------------------------------------------------------

func TestValidateHandler_ServiceInvalidInput_400(t *testing.T) {
	fv := &fakeValidator{err: apperrors.InvalidInput("translation_id must be numeric")}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=kodik&episode_id=1&translation_id=abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestValidateHandler_ServiceNotFound_404(t *testing.T) {
	fv := &fakeValidator{err: apperrors.NotFound("anime")}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=kodik&episode_id=1&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestValidateHandler_ServiceInternal_500(t *testing.T) {
	// Plain non-AppError → httputil.Error wraps as Internal → 500.
	fv := &fakeValidator{err: errors.New("db blew up")}
	router := newValidateRouter(NewInternalEpisodesValidateHandler(fv, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/anime/57466/episodes/validate?player=kodik&episode_id=1&translation_id=42", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
}
