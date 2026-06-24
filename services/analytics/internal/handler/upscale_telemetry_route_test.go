// Package handler_test holds the route-isolation test for UpscaleTelemetryHandler.
// It lives in an external test package (handler_test) so it can import both
// the handler package AND the transport package without triggering an import
// cycle (handler → transport → handler is a cycle; handler_test → both is not).
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/transport"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// noopSink satisfies handler.Sink (used by Collect/Effects/PlayerTelemetry).
type noopSink struct{}

func (noopSink) Enqueue(domain.Event) bool { return true }

// noopClientErrorSink satisfies handler.ClientErrorSink.
type noopClientErrorSink struct{}

func (noopClientErrorSink) Record(_ handler.WireClientError, _, _ string) {}

// noopEraser satisfies handler.Eraser.
type noopEraser struct{}

func (noopEraser) EraseByUserID(_ context.Context, _ string) error      { return nil }
func (noopEraser) EraseByAnonymousID(_ context.Context, _ string) error { return nil }

// captureUpscaleStore satisfies handler.UpscaleTelemetryStore and is used by
// the route isolation test (only needs routing to work, store calls not asserted).
type captureUpscaleStore struct {
	mu   sync.Mutex
	rows []repo.UpscaleTelemetryRow
}

func (s *captureUpscaleStore) InsertUpscaleTelemetry(_ context.Context, rows []repo.UpscaleTelemetryRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, rows...)
	return nil
}

// ── test ─────────────────────────────────────────────────────────────────────

// TestUpscaleTelemetryHandler_RouteIsolation constructs the REAL analytics
// router via transport.NewRouter and asserts:
//
//   (a) POST /internal/upscale-telemetry → 202  (the route IS registered —
//       the handler is wired at the internal-only path)
//   (b) POST /api/analytics/upscale-telemetry → 404  (the route is NOT on
//       any gateway-proxied public path — Docker-network-only, CD-15)
//
// Using the real router (not a hand-rolled mux) proves that the actual routing
// table enforces isolation, not just that the handler responds correctly when
// manually mounted.
func TestUpscaleTelemetryHandler_RouteIsolation(t *testing.T) {
	store := &captureUpscaleStore{}
	upscaleTelemetry := handler.NewUpscaleTelemetryHandler(store)

	sink := noopSink{}
	router := transport.NewRouter(
		handler.NewCollectHandler(sink, ""),
		handler.NewClientErrorHandler(noopClientErrorSink{}, ""),
		handler.NewPlayerTelemetryHandler(sink),
		handler.NewEffectsHandler(sink),
		handler.NewAdminHandler(noopEraser{}),
		nil, // readThresholds — optional, guarded by NewRouter's nil-check
		nil, // playerRanking  — optional, guarded by NewRouter's nil-check
		nil, // probe          — optional, guarded by NewRouter's nil-check
		upscaleTelemetry,
		logger.Default(),
		metrics.NewCollector("test-route-isolation"),
	)

	// (a) Internal path → 202 (real router has the route under /internal/*).
	req := httptest.NewRequest(http.MethodPost, "/internal/upscale-telemetry", strings.NewReader("[]"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("/internal/upscale-telemetry: expected 202 from real router, got %d", rec.Code)
	}

	// (b) Public gateway-proxied style path → 404 (no such route in real router).
	req2 := httptest.NewRequest(http.MethodPost, "/api/analytics/upscale-telemetry", strings.NewReader("[]"))
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("/api/analytics/upscale-telemetry: expected 404 from real router (not on public path), got %d", rec2.Code)
	}
}
