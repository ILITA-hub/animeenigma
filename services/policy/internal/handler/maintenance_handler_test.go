package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func seededMaintHandlers(t *testing.T) (*AdminMaintenanceHandler, *InternalMaintenanceHandler) {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.AutoMigrate(&domain.MaintenanceRoutine{})
	svc := service.NewMaintenanceService(repo.NewMaintenanceRepository(db), logger.Default())
	_ = svc.SeedDefaults(context.Background())
	return NewAdminMaintenanceHandler(svc, logger.Default()), NewInternalMaintenanceHandler(svc, logger.Default())
}

func TestAdminMaintenance_ListAndSet(t *testing.T) {
	admin, internal := seededMaintHandlers(t)

	rec := httptest.NewRecorder()
	admin.List(rec, httptest.NewRequest(http.MethodGet, "/api/admin/policy/maintenance/routines", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "maintenance_bot") {
		t.Errorf("list body missing maintenance_bot: %s", rec.Body.String())
	}

	// PUT enabled=false + settings; chi URL param must be injected.
	body := strings.NewReader(`{"enabled":false,"settings":{"model":"opus"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/policy/maintenance/routines/provider_recovery", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "provider_recovery")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec = httptest.NewRecorder()
	admin.SetRoutine(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set code = %d body=%s", rec.Code, rec.Body.String())
	}

	// Gate reflects the write.
	greq := httptest.NewRequest(http.MethodGet, "/internal/maintenance/routines/provider_recovery", nil)
	grctx := chi.NewRouteContext()
	grctx.URLParams.Add("id", "provider_recovery")
	greq = greq.WithContext(context.WithValue(greq.Context(), chi.RouteCtxKey, grctx))
	grec := httptest.NewRecorder()
	internal.Gate(grec, greq)
	var env struct {
		Data struct {
			Enabled  bool            `json:"enabled"`
			Settings json.RawMessage `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(grec.Body.Bytes(), &env); err != nil {
		t.Fatalf("gate decode: %v (%s)", err, grec.Body.String())
	}
	if env.Data.Enabled {
		t.Errorf("gate enabled=true; want false")
	}
	if !strings.Contains(string(env.Data.Settings), "opus") {
		t.Errorf("gate settings missing opus: %s", string(env.Data.Settings))
	}
}

// withID stamps a chi URL param "id" onto req (mirrors the RouteContext
// injection the router does for real requests) and returns the wrapped req.
func withID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// 1. Admin SetRoutine with a malformed body → 400 before the service is touched.
func TestAdminSetRoutine_invalidJSON(t *testing.T) {
	admin, _ := seededMaintHandlers(t)
	req := withID(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader("not json")), "provider_recovery")
	rec := httptest.NewRecorder()
	admin.SetRoutine(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d; want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// 2. Admin SetRoutine on an unknown id → 404 (liberrors.NotFound propagates
// through httputil.Error, not flattened to 500).
func TestAdminSetRoutine_unknownID(t *testing.T) {
	admin, _ := seededMaintHandlers(t)
	req := withID(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"enabled":true,"settings":{}}`)), "nope")
	rec := httptest.NewRecorder()
	admin.SetRoutine(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// 3. Admin SetRoutine with non-object settings → 400 (the service's
// isJSONObject rejection surfaces as InvalidInput → 400 via httputil.Error).
func TestAdminSetRoutine_nonObjectSettings(t *testing.T) {
	admin, _ := seededMaintHandlers(t)
	req := withID(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"enabled":true,"settings":null}`)), "provider_recovery")
	rec := httptest.NewRecorder()
	admin.SetRoutine(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d; want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// 4. Internal Gate on an unknown id → 404.
func TestInternalGate_unknownID(t *testing.T) {
	_, internal := seededMaintHandlers(t)
	req := withID(httptest.NewRequest(http.MethodGet, "/x", nil), "nope")
	rec := httptest.NewRecorder()
	internal.Gate(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// 5. Internal SetStatus happy path: 200, and it must NOT clobber enabled/settings
// — a subsequent Gate still decodes and reads back its seed defaults, and the
// admin List reflects the written LastSummary.
func TestInternalSetStatus_happyPathPreservesIntent(t *testing.T) {
	admin, internal := seededMaintHandlers(t)

	req := withID(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"ok":true,"summary":"synced"}`)), "git_autosync")
	rec := httptest.NewRecorder()
	internal.SetStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setstatus code = %d; want 200, body=%s", rec.Code, rec.Body.String())
	}

	// Gate still decodes cleanly (enabled/settings untouched by the status write).
	greq := withID(httptest.NewRequest(http.MethodGet, "/x", nil), "git_autosync")
	grec := httptest.NewRecorder()
	internal.Gate(grec, greq)
	if grec.Code != http.StatusOK {
		t.Fatalf("gate code = %d; want 200", grec.Code)
	}
	var genv struct {
		Data struct {
			Enabled  bool            `json:"enabled"`
			Settings json.RawMessage `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(grec.Body.Bytes(), &genv); err != nil {
		t.Fatalf("gate decode after setstatus: %v (%s)", err, grec.Body.String())
	}
	if !genv.Data.Enabled {
		t.Errorf("setstatus clobbered enabled: git_autosync seed default is true, got false")
	}

	// LastSummary is observable through the admin List projection.
	lrec := httptest.NewRecorder()
	admin.List(lrec, httptest.NewRequest(http.MethodGet, "/api/admin/policy/maintenance/routines", nil))
	var lenv struct {
		Data struct {
			Routines []domain.MaintenanceRoutine `json:"routines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(lrec.Body.Bytes(), &lenv); err != nil {
		t.Fatalf("list decode: %v (%s)", err, lrec.Body.String())
	}
	var found bool
	for _, r := range lenv.Data.Routines {
		if r.ID == "git_autosync" {
			found = true
			if r.LastSummary != "synced" {
				t.Errorf("LastSummary = %q; want %q", r.LastSummary, "synced")
			}
			if r.LastOK == nil || !*r.LastOK {
				t.Errorf("LastOK = %v; want true", r.LastOK)
			}
		}
	}
	if !found {
		t.Fatalf("git_autosync missing from list: %s", lrec.Body.String())
	}
}

// 6. Internal SetStatus on an unknown id → 404.
func TestInternalSetStatus_unknownID(t *testing.T) {
	_, internal := seededMaintHandlers(t)
	req := withID(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"ok":true,"summary":"x"}`)), "nope")
	rec := httptest.NewRecorder()
	internal.SetStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404, body=%s", rec.Code, rec.Body.String())
	}
}
