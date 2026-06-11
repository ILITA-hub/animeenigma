package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubInternalListService satisfies the unexported listInternalService interface
// in the handler package via structural typing — it has the same single method.
// Tests use it to count handler invocations from the router without spinning
// up a real GORM DB or service.
type stubInternalListService struct {
	mu    sync.Mutex
	calls int
}

func (s *stubInternalListService) GetUserListByStatusesWithProgress(
	_ context.Context, _ string, _ []string,
) ([]domain.InternalListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return []domain.InternalListItem{}, nil
}

func (s *stubInternalListService) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// newPlayerRouter builds a NewRouter() with a stub InternalListHandler wired
// in and nils everywhere else. The other handlers are not exercised by the
// /internal/users/{user_id}/list-focused tests below, so passing nil is
// safe — chi only invokes a route's handler when that route is hit.
func newPlayerRouter(t *testing.T, internalListHandler *handler.InternalListHandler) http.Handler {
	t.Helper()
	// Bare-bones logger so RequestLogger middleware doesn't blow up.
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)

	// JWTConfig zero-value is acceptable — AuthMiddleware is only applied
	// inside /api/users which these tests never hit. The validation path
	// is never exercised.
	router := NewRouter(
		nil, // progressHandler
		nil, // listHandler
		nil, // historyHandler
		nil, // reviewHandler
		nil, // commentHandler
		nil, // malImportHandler
		nil, // malExportHandler
		nil, // shikimoriImportHandler
		nil, // reportHandler
		nil, // syncHandler
		nil, // activityHandler
		nil, // exportHandler
		nil, // preferenceHandler
		nil, // overrideHandler
		nil, // adminReportsHandler
		internalListHandler,
		nil, // viewerContextHandler
		zeroJWTConfig(),
		log,
		zeroMetricsCollector(t),
	)
	return router
}

// TestRouter_InternalListMounted_OutsideAPI asserts that the new internal
// endpoint is reachable WITHOUT the /api prefix. The router MUST treat
// /internal/* as peers of /health and /metrics (mounted at the root) — not
// as a subtree of /api.
func TestRouter_InternalListMounted_OutsideAPI(t *testing.T) {
	stub := &stubInternalListService{}
	log, _ := logger.New(logger.Config{Level: "error", Development: false})
	h := handler.NewInternalListHandlerFromService(stub, log)
	router := newPlayerRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		"GET /internal/users/{user_id}/list MUST be mounted at the root (peer of /health, /metrics) and return 200")
	assert.Equal(t, 1, stub.Calls(), "service must be invoked when the route is hit")
}

// TestRouter_InternalListNotInsideAPI asserts the /internal route is NOT
// shadowed under /api — a request to /api/internal/users/... must 404.
// This is the structural guard that keeps the gateway from accidentally
// proxying the internal endpoint via its catch-all /api/* rule.
func TestRouter_InternalListNotInsideAPI(t *testing.T) {
	stub := &stubInternalListService{}
	log, _ := logger.New(logger.Config{Level: "error", Development: false})
	h := handler.NewInternalListHandlerFromService(stub, log)
	router := newPlayerRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code,
		"GET /api/internal/users/{user_id}/list MUST 404 — the route must not be shadowed inside /api")
	assert.Equal(t, 0, stub.Calls(), "service must not be invoked for /api-prefixed path")
}

// TestRouter_InternalList_NoAuthRequired asserts that a request without
// any Authorization header reaches the stub handler — i.e. there is NO
// AuthMiddleware on the route (docker-network trust boundary per
// CONTEXT.md `<decisions>` "Player internal endpoints").
func TestRouter_InternalList_NoAuthRequired(t *testing.T) {
	stub := &stubInternalListService{}
	log, _ := logger.New(logger.Config{Level: "error", Development: false})
	h := handler.NewInternalListHandlerFromService(stub, log)
	router := newPlayerRouter(t, h)

	// Bare request — no Authorization header, no Cookie, nothing.
	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusUnauthorized, rr.Code,
		"internal endpoint MUST NOT emit 401 — no auth middleware is attached")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 1, stub.Calls(), "handler MUST be reached without any auth")
}

// TestRouter_InternalListNotNil_RouteRegistered asserts the route registration
// is gated by a non-nil internalListHandler check (mirroring the catalog
// precedent for the optional internal handlers in
// services/catalog/internal/transport/router.go). When the handler is nil,
// the route MUST NOT exist (404), so the player binary can still boot in
// configurations that disable the spotlight feature.
func TestRouter_InternalListNotNil_RouteRegistered(t *testing.T) {
	router := newPlayerRouter(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=watching", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code,
		"with nil internalListHandler, /internal/users/{user_id}/list MUST 404 (route guarded by nil-check)")
}

// TestRouter_RouteOrder_InternalBeforeAPI is a textual structural assertion
// that the internal route registration appears BEFORE the r.Route("/api"
// block in router.go. The runtime tests above already cover the behavior;
// this one guards the source order so a future refactor that accidentally
// nests /internal under /api is caught at unit-test time.
//
// We assert via the source file itself rather than reflecting on the chi
// router (chi exposes no public mounting order API).
func TestRouter_RouteOrder_InternalBeforeAPI(t *testing.T) {
	src := readRouterSource(t)
	internalIdx := strings.Index(src, `"/internal/users/{user_id}/list"`)
	apiIdx := strings.Index(src, `r.Route("/api"`)
	require.NotEqual(t, -1, internalIdx, "router.go must register /internal/users/{user_id}/list")
	require.NotEqual(t, -1, apiIdx, "router.go must contain r.Route(\"/api\"")
	assert.Less(t, internalIdx, apiIdx,
		"the /internal route MUST be registered before the r.Route(\"/api\" block")
}
