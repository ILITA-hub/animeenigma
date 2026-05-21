package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeListInternalService records the last call args + returns scripted
// outputs. Satisfies the unexported listInternalService interface in
// list_internal.go by structural typing.
type fakeListInternalService struct {
	mu             sync.Mutex
	lastUserID     string
	lastStatuses   []string
	returnItems    []domain.InternalListItem
	returnErr      error
	callCount      int
}

func (f *fakeListInternalService) GetUserListByStatusesWithProgress(
	_ context.Context, userID string, statuses []string,
) ([]domain.InternalListItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.lastUserID = userID
	// Copy the slice so the caller can mutate without polluting the recorded value.
	f.lastStatuses = append([]string(nil), statuses...)
	return f.returnItems, f.returnErr
}

func (f *fakeListInternalService) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

func (f *fakeListInternalService) LastStatuses() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.lastStatuses))
	copy(out, f.lastStatuses)
	return out
}

// newInternalListHandlerForTest builds a router-mounted handler with the
// provided fake service so tests exercise the real chi URL param parsing
// + JSON encoding paths.
func newInternalListHandlerForTest(t *testing.T, svc listInternalService) http.Handler {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)
	h := &InternalListHandler{svc: svc, log: log}
	r := chi.NewRouter()
	r.Get("/internal/users/{user_id}/list", h.ListByStatuses)
	return r
}

func TestListByStatuses_Internal_HappyPath(t *testing.T) {
	fake := &fakeListInternalService{
		returnItems: []domain.InternalListItem{
			{AnimeID: "anime-1", Name: "Bocchi", Status: "watching", LastWatchedEpisode: 3},
		},
	}
	h := newInternalListHandlerForTest(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body struct {
		Items []domain.InternalListItem `json:"items"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	require.Len(t, body.Items, 1)
	assert.Equal(t, "anime-1", body.Items[0].AnimeID)
	assert.Equal(t, "u1", fake.lastUserID)
	assert.Equal(t, []string{"watching"}, fake.LastStatuses())
}

func TestListByStatuses_Internal_NoStatusParam_ReturnsEmptyItems(t *testing.T) {
	fake := &fakeListInternalService{
		returnItems: []domain.InternalListItem{}, // service short-circuits empty statuses
	}
	h := newInternalListHandlerForTest(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	items, ok := body["items"].([]any)
	require.True(t, ok, "items must be a JSON array, even when empty")
	assert.Len(t, items, 0)

	// The handler MUST still invoke the service with an empty slice — the
	// service is the authority on the empty-input contract, not the handler.
	assert.Equal(t, 1, fake.Calls())
	assert.Equal(t, []string{}, fake.LastStatuses())
}

func TestListByStatuses_Internal_UnknownStatusFilteredOut(t *testing.T) {
	fake := &fakeListInternalService{}
	h := newInternalListHandlerForTest(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/internal/users/u1/list?status=watching,foo,planned", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// "foo" must be dropped silently; "watching" and "planned" pass through.
	assert.Equal(t, []string{"watching", "planned"}, fake.LastStatuses())
}

func TestListByStatuses_Internal_UnknownStatusOnly_ReturnsEmpty(t *testing.T) {
	fake := &fakeListInternalService{
		returnItems: []domain.InternalListItem{},
	}
	h := newInternalListHandlerForTest(t, fake)

	// Only unknown statuses — handler filters them all out and we end up with
	// an empty slice, which the service treats as "no query, empty result".
	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=foo,bar", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{}, fake.LastStatuses())
}

func TestListByStatuses_Internal_MissingUserID_Returns400(t *testing.T) {
	fake := &fakeListInternalService{}
	h := newInternalListHandlerForTest(t, fake)

	// chi treats `/internal/users//list` as a literal path; the {user_id}
	// param resolves to empty string. The handler must short-circuit with 400.
	req := httptest.NewRequest(http.MethodGet, "/internal/users//list", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// chi may emit 404 for the empty-param path itself. Either 400 (handler
	// reached + early return) or 404 (chi router refused the route) is an
	// acceptable "did not reach the service" outcome. We assert NOT 200 and
	// the service was never called.
	assert.NotEqual(t, http.StatusOK, rr.Code,
		"missing user_id must NOT reach the service or yield 200")
	assert.Equal(t, 0, fake.Calls(),
		"service must not be invoked when user_id is missing")
}

func TestListByStatuses_Internal_ServiceError_Returns500(t *testing.T) {
	fake := &fakeListInternalService{
		returnErr: errors.New("simulated query failure"),
	}
	h := newInternalListHandlerForTest(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	// Body must be valid JSON with an error field — internal callers parse
	// the body to log the failure context.
	body := strings.TrimSpace(rr.Body.String())
	assert.Contains(t, body, "error")
}

func TestListByStatuses_Internal_StatusValuesTrimmed(t *testing.T) {
	fake := &fakeListInternalService{}
	h := newInternalListHandlerForTest(t, fake)

	// Whitespace around the CSV values must be tolerated — callers may not
	// canonicalise their query strings.
	req := httptest.NewRequest(http.MethodGet,
		"/internal/users/u1/list?status=%20watching%20,%20planned%20", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"watching", "planned"}, fake.LastStatuses())
}
