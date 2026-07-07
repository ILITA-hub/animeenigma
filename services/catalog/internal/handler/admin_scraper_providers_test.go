package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAdminScraperProvidersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// reqWithNameParam builds a request carrying a chi URLParam "name" (mirrors the
// chi.NewRouteContext idiom in internal_episodes_test.go doReq).
func reqWithNameParam(method, target, name string, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestAdminScraperProviders_List verifies the admin List handler returns the
// seeded providers with policy/health/derived_state populated, and that
// derived_state matches domain.ScraperProvider.DerivedState() for a known
// (policy,health) pair.
func TestAdminScraperProviders_List(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	if err := db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, Group: "en", ScraperOperated: true},
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthRecovering, Group: "en", ScraperOperated: true},
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest(http.MethodGet, "/api/admin/scraper-providers", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	var body struct {
		Data struct {
			Providers []map[string]any `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}
	if len(body.Data.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d", len(body.Data.Providers))
	}

	got := map[string]map[string]any{}
	for _, p := range body.Data.Providers {
		got[p["name"].(string)] = p
	}

	gogo := got["gogoanime"]
	if gogo["policy"] != "auto" || gogo["health"] != "up" {
		t.Fatalf("gogoanime policy/health = %v/%v, want auto/up", gogo["policy"], gogo["health"])
	}
	if gogo["derived_state"] != domain.StateUP {
		t.Fatalf("gogoanime derived_state = %v, want %v", gogo["derived_state"], domain.StateUP)
	}

	allanime := got["allanime"]
	if allanime["policy"] != "manual" || allanime["health"] != "recovering" {
		t.Fatalf("allanime policy/health = %v/%v, want manual/recovering", allanime["policy"], allanime["health"])
	}
	if allanime["derived_state"] != domain.StateRecovering {
		t.Fatalf("allanime derived_state = %v, want %v", allanime["derived_state"], domain.StateRecovering)
	}
}

// TestAdminScraperProviders_SetPolicy_Disabled verifies flipping a provider to
// disabled: 200, Policy==disabled, PolicySince advances, Health/HealthSince
// stay untouched (health is probe-owned).
func TestAdminScraperProviders_SetPolicy_Disabled(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	originalPolicySince := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	originalHealthSince := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := db.Create(&domain.ScraperProvider{
		Name:        "gogoanime",
		Policy:      domain.PolicyAuto,
		Health:      domain.HealthUp,
		PolicySince: originalPolicySince,
		HealthSince: originalHealthSince,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	body, _ := json.Marshal(map[string]string{"policy": "disabled"})
	req := reqWithNameParam(http.MethodPut, "/api/admin/scraper-providers/gogoanime/policy", "gogoanime", body)
	rr := httptest.NewRecorder()
	h.SetPolicy(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	var row domain.ScraperProvider
	if err := db.Where("name = ?", "gogoanime").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Policy != domain.PolicyDisabled {
		t.Fatalf("Policy = %q, want disabled", row.Policy)
	}
	if !row.PolicySince.After(originalPolicySince) {
		t.Fatalf("PolicySince did not advance: %v (was %v)", row.PolicySince, originalPolicySince)
	}
	if row.Health != domain.HealthUp {
		t.Fatalf("Health = %q, want unchanged up", row.Health)
	}
	if !row.HealthSince.Equal(originalHealthSince) {
		t.Fatalf("HealthSince changed: %v (was %v)", row.HealthSince, originalHealthSince)
	}

	var respBody struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}
	if respBody.Data["policy"] != "disabled" {
		t.Fatalf("response policy = %v, want disabled", respBody.Data["policy"])
	}
	if respBody.Data["derived_state"] != domain.StateDisabled {
		t.Fatalf("response derived_state = %v, want %v", respBody.Data["derived_state"], domain.StateDisabled)
	}
}

// TestAdminScraperProviders_SetPolicy_Auto verifies flipping back to auto.
func TestAdminScraperProviders_SetPolicy_Auto(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	if err := db.Create(&domain.ScraperProvider{
		Name:   "gogoanime",
		Policy: domain.PolicyDisabled,
		Health: domain.HealthDown,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	body, _ := json.Marshal(map[string]string{"policy": "auto"})
	req := reqWithNameParam(http.MethodPut, "/api/admin/scraper-providers/gogoanime/policy", "gogoanime", body)
	rr := httptest.NewRecorder()
	h.SetPolicy(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var row domain.ScraperProvider
	if err := db.Where("name = ?", "gogoanime").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Policy != domain.PolicyAuto {
		t.Fatalf("Policy = %q, want auto", row.Policy)
	}
}

// TestAdminScraperProviders_SetPolicy_UnknownName verifies 404 for a name that
// doesn't exist in stream_providers.
func TestAdminScraperProviders_SetPolicy_UnknownName(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	body, _ := json.Marshal(map[string]string{"policy": "auto"})
	req := reqWithNameParam(http.MethodPut, "/api/admin/scraper-providers/nope/policy", "nope", body)
	rr := httptest.NewRecorder()
	h.SetPolicy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rr.Code, rr.Body.String())
	}
}

// TestAdminScraperProviders_SetPolicy_RejectsManual verifies manual is a
// machine-set state and NOT an admin lever — same rejection path as any other
// invalid value.
func TestAdminScraperProviders_SetPolicy_RejectsManual(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	if err := db.Create(&domain.ScraperProvider{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp}).Error; err != nil {
		t.Fatal(err)
	}
	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	body, _ := json.Marshal(map[string]string{"policy": "manual"})
	req := reqWithNameParam(http.MethodPut, "/api/admin/scraper-providers/gogoanime/policy", "gogoanime", body)
	rr := httptest.NewRecorder()
	h.SetPolicy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
	}

	var row domain.ScraperProvider
	if err := db.Where("name = ?", "gogoanime").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Policy != domain.PolicyAuto {
		t.Fatalf("Policy should be unchanged, got %q", row.Policy)
	}
}

// TestAdminScraperProviders_SetPolicy_RejectsBogus verifies an arbitrary
// invalid value is rejected with 400.
func TestAdminScraperProviders_SetPolicy_RejectsBogus(t *testing.T) {
	db := newAdminScraperProvidersTestDB(t)
	if err := db.Create(&domain.ScraperProvider{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp}).Error; err != nil {
		t.Fatal(err)
	}
	h := handler.NewAdminScraperProvidersHandler(db, testNopLogger())
	body, _ := json.Marshal(map[string]string{"policy": "bogus"})
	req := reqWithNameParam(http.MethodPut, "/api/admin/scraper-providers/gogoanime/policy", "gogoanime", body)
	rr := httptest.NewRecorder()
	h.SetPolicy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
	}
}
