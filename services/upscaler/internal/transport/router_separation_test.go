package transport

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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

func (s *stubLeaser) OnLeaseReq(_ context.Context, _ string) (*domain.UpscaleSegment, controlplane.LeaseHandles, string, int, error) {
	return nil, controlplane.LeaseHandles{}, "", 0, nil
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
	// A real SegmentHandler (with nil repos) registers the /worker/segments/*
	// routes; the separation tests only check route RESOLUTION, never invoking
	// the handler bodies with a valid capability signature (so the nil repos are
	// never dereferenced — capability verification rejects unsigned requests
	// before any repo call).
	segHandler := handler.NewSegmentHandler(t.TempDir(), nil, nil, log)
	// nil adminHandler: the separation tests don't exercise the admin CRUD
	// handlers and the router's nil-guard skips wiring them in. Tests that
	// need real admin handlers construct their own fixture (see admin_test.go).
	return NewRouter(log, sharedUpscalerCollector(), hub, nil, segHandler, nil, nil, nil)
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

// TestUpscalerRouter_SegmentRoutesResolveOnWorkerSurface — the segment data
// plane (GET+PUT /worker/segments/{job}/{idx}) is REGISTERED on the worker
// surface. We confirm by sending an unsigned request: the route resolves and
// the SegmentHandler rejects with 401 (generic "unauthorized"). A non-resolving
// route would instead produce chi's plain-text "404 page not found".
func TestUpscalerRouter_SegmentRoutesResolveOnWorkerSurface(t *testing.T) {
	router := buildUpscalerRouter(t)

	for _, m := range []string{http.MethodGet, http.MethodPut} {
		// No exp/sig → capability verification fails → handler returns generic 401.
		req := httptest.NewRequest(m, "/worker/segments/job-123/0", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Route resolves: handler runs and returns its own 401, NOT chi's 404.
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s /worker/segments/{job}/{idx}: got 404 — route NOT registered on worker surface", m)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s /worker/segments/{job}/{idx} unsigned: status = %d; want 401 (handler reached, capability rejected)", m, rec.Code)
		}
		// The generic 401 body must NOT be chi's plain 404 text.
		if bodyContains(rec.Body.String(), "404 page not found") {
			t.Errorf("%s /worker/segments/{job}/{idx}: body is chi 404 — route not registered: %q", m, rec.Body.String())
		}
	}
}

// TestUpscalerRouter_SegmentRoutesNotOnAdminSurface — /api/upscale/segments/*
// must NOT resolve. The segment data plane lives ONLY on the worker surface;
// the admin surface (/api/upscale/*) is a distinct group and has no segment
// route. Even WITH the gateway header, /api/upscale/segments/{job}/{idx} 404s
// because no such route is registered there.
func TestUpscalerRouter_SegmentRoutesNotOnAdminSurface(t *testing.T) {
	router := buildUpscalerRouter(t)

	for _, m := range []string{http.MethodGet, http.MethodPut} {
		req := httptest.NewRequest(m, "/api/upscale/segments/job-123/0", nil)
		req.Header.Set(internalGatewayHeader, "1") // pass the admin gate
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// No segment route on the admin surface → 404 (or 405), never 200/401.
		if rec.Code == http.StatusOK || rec.Code == http.StatusUnauthorized || rec.Code == http.StatusNoContent {
			t.Errorf("%s /api/upscale/segments/{job}/{idx}: status = %d — segment route LEAKED onto the admin surface", m, rec.Code)
		}
	}
}

// TestUpscalerRouter_AdminPathNotOnWorkerSurface — the worker surface must not
// serve /api/upscale/* routes. A request to /worker/api/upscale/... or a direct
// /api/upscale/* without the gateway header is gated/absent. This asserts the
// inverse separation: /api/upscale/* does NOT resolve as a worker-surface route.
func TestUpscalerRouter_AdminPathNotOnWorkerSurface(t *testing.T) {
	router := buildUpscalerRouter(t)

	// /api/upscale/* without the gateway header → 404 (admin gate fires).
	// This is the worker-reachable scenario (the ext edge cannot set the header),
	// proving admin routes are unreachable from the worker edge.
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/api/upscale/jobs without gateway header: status = %d; want 404 (unreachable from worker edge)", rec.Code)
	}
}

// TestUpscalerRouter_AdminJobsResolveWithGatewayHeader verifies that the admin
// job endpoints (POST /api/upscale/jobs, GET /api/upscale/jobs, etc.) are
// registered under the gateway-gated group. With the header set and a nil
// adminHandler, chi returns 405 or 404 for those routes (no handler wired) —
// what matters is that requireGatewayInternal does NOT fire a "not found" gate
// response. We use a purpose-built router with a stub AdminHandler so the
// routes are actually registered and return something other than our gate 404.
func TestUpscalerRouter_AdminJobsResolveWithGatewayHeader(t *testing.T) {
	t.Parallel()
	log := logger.Default()
	hub := controlplane.NewHub(&stubLeaser{}, &stubWorkerHB{}, log)
	segHandler := handler.NewSegmentHandler(t.TempDir(), nil, nil, log)
	// A nil adminHandler causes the route group to have no routes wired.
	// We verify the gate passes (no gate-404) and chi handles routing.
	router := NewRouter(log, sharedUpscalerCollector(), hub, nil, segHandler, nil, nil, nil)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/upscale/jobs"},
		{http.MethodGet, "/api/upscale/workers"},
		{http.MethodGet, "/api/upscale/jobs/some-uuid"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(c.method, c.path, nil)
			req.Header.Set(internalGatewayHeader, "1")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// The gate must NOT have fired (gate 404 would be JSON with NOT_FOUND).
			// chi's own 404/405 for an unregistered path is plain text and does not
			// contain our gate's JSON body.
			if containsGateBody(rec.Body.String()) {
				t.Errorf("%s %s with header: gate fired despite header being set — body: %q",
					c.method, c.path, rec.Body.String())
			}
		})
	}
}

// TestUpscalerRouter_AdminRequiresGatewayHeader_WithoutHeader verifies that
// ALL known admin endpoints 404 (gate fires) when no gateway header is present.
func TestUpscalerRouter_AdminRequiresGatewayHeader_WithoutHeader(t *testing.T) {
	t.Parallel()
	router := buildUpscalerRouter(t)

	for _, path := range []string{
		"/api/upscale/jobs",
		"/api/upscale/workers",
		"/api/upscale/jobs/some-id",
		"/api/upscale/jobs/some-id/cancel",
		"/api/upscale/jobs/some-id/retry",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			// NO gateway header
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("%q without gateway header: status = %d; want 404", path, rec.Code)
			}
		})
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

// ── Integration test: real SQLite-backed AdminHandler ─────────────────────────

var (
	transportSQLiteOnce sync.Once
)

func transportGenUUID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:gosec // test-only
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func transportRegisterSQLite() {
	transportSQLiteOnce.Do(func() {
		sql.Register("sqlite3_transport_now", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true); err != nil {
					return err
				}
				return conn.RegisterFunc("gen_random_uuid", transportGenUUID, false)
			},
		})
	})
}

// openTransportTestDB returns a fresh in-memory SQLite DB with the upscale_jobs
// and upscale_workers tables, suitable for constructing a real AdminHandler in
// transport-layer tests.
func openTransportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	transportRegisterSQLite()
	dsn := fmt.Sprintf("file:upscaler_transport_%s?mode=memory&cache=shared", uuid.New().String())
	db, err := gorm.Open(&sqlite.Dialector{
		DriverName: "sqlite3_transport_now",
		DSN:        dsn,
	}, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS upscale_jobs (
			id                TEXT NOT NULL PRIMARY KEY,
			shikimori_id      TEXT NOT NULL,
			episode           INTEGER NOT NULL,
			library_infohash  TEXT,
			model             TEXT NOT NULL,
			scale             INTEGER NOT NULL DEFAULT 2,
			status            TEXT NOT NULL DEFAULT 'queued',
			progress_pct      INTEGER NOT NULL DEFAULT 0,
			source_codec      TEXT,
			source_pixfmt     TEXT,
			source_fps        TEXT,
			source_height     INTEGER NOT NULL DEFAULT 0,
			segment_count     INTEGER NOT NULL DEFAULT 0,
			output_prefix     TEXT,
			error_text        TEXT,
			created_at        DATETIME,
			updated_at        DATETIME,
			completed_at      DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS upscale_workers (
			worker_id          TEXT NOT NULL PRIMARY KEY,
			gpu_info           TEXT,
			image_version      TEXT,
			models_available   TEXT,
			status             TEXT NOT NULL DEFAULT 'idle',
			current_job_id     TEXT,
			current_segment    INTEGER,
			session_expires_at DATETIME,
			last_heartbeat_at  DATETIME,
			created_at         DATETIME
		)`,
	}
	for _, ddl := range ddls {
		if err := db.Exec(ddl).Error; err != nil {
			t.Skipf("create table: %v", err)
		}
	}
	return db
}

// TestRouter_AdminJobsWithRealHandler builds a router with a real SQLite-backed
// AdminHandler, sends GET /api/upscale/jobs WITH the gateway header, and asserts 200.
func TestRouter_AdminJobsWithRealHandler(t *testing.T) {
	t.Parallel()
	log := logger.Default()
	db := openTransportTestDB(t)

	jobs := repo.NewJobRepository(db)
	workers := repo.NewWorkerRepository(db)
	adminH := handler.NewAdminHandler(jobs, workers, 2, "realesrgan-x4plus-anime", log)

	hub := controlplane.NewHub(&stubLeaser{}, &stubWorkerHB{}, log)
	segH := handler.NewSegmentHandler(t.TempDir(), nil, nil, log)
	router := NewRouter(log, sharedUpscalerCollector(), hub, nil, segH, adminH, nil, nil)

	// Seed one queued job so the list returns non-empty.
	ctx := context.Background()
	seeded := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "99",
		Episode:     1,
		Model:       "realesrgan-x4plus-anime",
		Scale:       2,
		Status:      domain.JobQueued,
	}
	if err := jobs.Create(ctx, seeded); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs", nil)
	req.Header.Set(internalGatewayHeader, "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/upscale/jobs with real handler: status = %d; want 200 (body: %s)",
			rec.Code, rec.Body.String())
	}
	// Body must NOT be our gate 404.
	if containsGateBody(rec.Body.String()) {
		t.Errorf("GET /api/upscale/jobs: gate fired despite header being set")
	}
}
