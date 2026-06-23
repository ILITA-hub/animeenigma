package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

// shared collector for this test file — avoids duplicate registration panics.
var (
	upscalerTestCollector     *metrics.Collector
	upscalerTestCollectorOnce sync.Once
)

func sharedUpscalerCollector() *metrics.Collector {
	upscalerTestCollectorOnce.Do(func() {
		upscalerTestCollector = metrics.NewCollector("upscaler-router-test")
	})
	return upscalerTestCollector
}

// stubLeaser satisfies controlplane.Leaser but returns nothing (no jobs).
type stubLeaser struct{}

func (s *stubLeaser) OnLeaseReq(_ context.Context, _ string) (*domain.UpscaleSegment, controlplane.LeaseHandles, error) {
	return nil, controlplane.LeaseHandles{}, nil
}

// stubWorkerHB satisfies controlplane.WorkerHeartbeater (no-op).
type stubWorkerHB struct{}

func (s *stubWorkerHB) Heartbeat(_ context.Context, _, _ string, _ int, _ time.Time) error {
	return nil
}

func buildUpscalerRouter(t *testing.T) http.Handler {
	t.Helper()
	log := logger.Default()
	hub := controlplane.NewHub(&stubLeaser{}, &stubWorkerHB{}, log)
	// nil GormEnrollStore is fine for separation tests — no enroll calls are made.
	return NewRouter(log, sharedUpscalerCollector(), hub, nil)
}

// TestUpscalerRouter_WorkerSurfaceReachable — /worker/* routes exist on the
// upscaler router (even if handlers are stubs). They should NOT 404 because
// they're registered (chi returns 405 Method Not Allowed for a registered
// route hit with the wrong method, and plain 404 only for unregistered paths).
// Since the handlers are placeholders, they return 404 from the empty group —
// but the important thing is the ADMIN route returns 404 when no gateway
// header is set (see the next test).
func TestUpscalerRouter_AdminRequiresGatewayHeader(t *testing.T) {
	router := buildUpscalerRouter(t)

	// Direct call to /api/upscale/* WITHOUT the gateway-injected header
	// must return 404 (we deliberately obscure the existence of the gate).
	for _, path := range []string{
		"/api/upscale/jobs",
		"/api/upscale/jobs/some-id",
		"/api/upscale/status",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// 404 expected — no header → gate fires → not found.
		if rec.Code != http.StatusNotFound {
			t.Errorf("path %q without gateway header: status = %d; want 404", path, rec.Code)
		}
	}
}

// TestUpscalerRouter_AdminAccessibleWithGatewayHeader — with the gateway
// internal header set, the admin route group is accessible. (The group is
// empty — placeholder routes return 404 from chi, not from our gate.)
func TestUpscalerRouter_AdminAccessibleWithGatewayHeader(t *testing.T) {
	router := buildUpscalerRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs", nil)
	// Simulate the gateway-injected header.
	req.Header.Set(internalGatewayHeader, "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// With the header present, requireGatewayInternal passes. The route group
	// is empty (placeholder), so chi returns 404 — but NOT our gate's 404.
	// Both are 404, but what matters is that our gate did NOT fire (i.e.,
	// we got past the middleware). We verify by checking that the response
	// body does NOT contain "not found" from our custom gate response.
	// (Our gate writes httputil.NotFound which encodes {"error":{"code":"...","message":"not found"}}).
	// Chi's own 404 writes "404 page not found\n" without JSON.
	body := rec.Body.String()
	if rec.Code == http.StatusNotFound && containsGateBody(body) {
		t.Errorf("gateway header set but gate still fired — header not recognized: %q", body)
	}
}

// TestUpscalerRouter_WorkerVsAdminSeparation — the worker surface (/worker/*)
// and the admin surface (/api/upscale/*) are distinct groups. A request that
// hits /worker/* does not gate on the X-Gateway-Internal header (only the
// admin group gates on it). This verifies the two surfaces are truly separate.
func TestUpscalerRouter_WorkerVsAdminSeparation(t *testing.T) {
	router := buildUpscalerRouter(t)

	// Worker path WITHOUT the gateway header — should NOT 404 from our gate.
	// (The placeholder route group returns 404 from chi for unregistered
	// sub-paths, but that's chi's 404, not our gate's 404.)
	req := httptest.NewRequest(http.MethodPost, "/worker/enroll", nil)
	// Deliberately do NOT set X-Gateway-Internal.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Our admin gate must NOT have fired for /worker/*.
	body := rec.Body.String()
	if containsGateBody(body) {
		t.Errorf("/worker/enroll hit the admin gate (requireGatewayInternal) — worker and admin surfaces are not separated: %q", body)
	}
}

// TestUpscalerRouter_HealthReachableWithoutHeader — /health is not gated and
// must be reachable from any caller (Docker healthcheck, ops probes, gateway
// status aggregator).
func TestUpscalerRouter_HealthReachableWithoutHeader(t *testing.T) {
	router := buildUpscalerRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/health: status = %d; want 200", rec.Code)
	}
}

// containsGateBody reports whether the body looks like it came from our
// requireGatewayInternal gate (httputil.NotFound encodes JSON).
func containsGateBody(body string) bool {
	// Our gate calls httputil.NotFound(w, "not found") which produces
	// {"success":false,"error":{"code":"NOT_FOUND","message":"not found"}}
	return len(body) > 0 &&
		(bodyContains(body, `"NOT_FOUND"`) || bodyContains(body, `"not found"`))
}

func bodyContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
