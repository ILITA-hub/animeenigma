package handler_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testProviderPolicyCfg() config.ProviderPolicyConfig {
	return config.ProviderPolicyConfig{
		DemoteAfter:  24 * time.Hour,
		PromoteAfter: 24 * time.Hour,
		Cadence: domain.CadenceConfig{
			Up:               6 * time.Hour,
			Recovering:       12 * time.Hour,
			Manual:           24 * time.Hour,
			RecoveringSample: 3,
			FullSample:       5,
		},
	}
}

func TestProbeResult_FlipsRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-48 * time.Hour)
	if err := db.Create(&domain.ScraperProvider{
		Name:            "gogoanime",
		Policy:          domain.PolicyAuto,
		Health:          domain.HealthDown,
		HealthSince:     t0,
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"gogoanime","pass":false,"reason":"status_403"}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "gogoanime")
	if p.Policy != domain.PolicyManual { // down >24h + fail -> demote
		t.Fatalf("policy=%s want manual", p.Policy)
	}
	if p.LastProbedAt.IsZero() {
		t.Fatal("last_probed_at not stamped")
	}
}

func TestProbeResult_SkipsDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name:            "nineanime",
		Policy:          domain.PolicyDisabled,
		Health:          domain.HealthDown,
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"nineanime","pass":false}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	// Policy must remain disabled — we skipped mutation.
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "nineanime")
	if p.Policy != domain.PolicyDisabled {
		t.Fatalf("policy=%s want disabled (should have skipped)", p.Policy)
	}
}

func TestProbeResult_UnknownProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"doesnotexist","pass":true}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 404 {
		t.Fatalf("code=%d want 404", rr.Code)
	}
}
