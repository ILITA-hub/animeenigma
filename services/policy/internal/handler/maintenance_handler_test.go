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
