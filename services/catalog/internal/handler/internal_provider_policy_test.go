package handler_test

import (
	"encoding/json"
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

// newHandlerTestDB returns an in-memory SQLite DB for handler unit tests.
func newHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

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

func TestProbePlan_DueSet(t *testing.T) {
	db := newHandlerTestDB(t)
	db.AutoMigrate(&domain.ScraperProvider{})
	now := time.Now().UTC()
	db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-7 * time.Hour), ScraperOperated: true},    // due (6h cadence for Up)
		{Name: "miruro", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-1 * time.Hour), ScraperOperated: true},        // not due
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthDown, LastProbedAt: now.Add(-25 * time.Hour), ScraperOperated: true}, // due (24h manual cadence), size=1, ff=true
		{Name: "deadguy", Policy: domain.PolicyDisabled, Health: domain.HealthDown, LastProbedAt: now.Add(-99 * time.Hour), ScraperOperated: true}, // excluded
	})
	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	h.ProbePlan(rr, httptest.NewRequest("GET", "/internal/providers/probe-plan", nil))

	var body struct {
		Data struct {
			Plan []struct {
				Provider   string `json:"provider"`
				SampleSize int    `json:"sample_size"`
				FailFast   bool   `json:"fail_fast"`
			} `json:"plan"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &body)
	got := map[string]struct {
		size int
		ff   bool
	}{}
	for _, e := range body.Data.Plan {
		got[e.Provider] = struct {
			size int
			ff   bool
		}{e.SampleSize, e.FailFast}
	}
	if _, ok := got["miruro"]; ok {
		t.Fatal("miruro not due — should be absent")
	}
	if _, ok := got["deadguy"]; ok {
		t.Fatal("disabled must be excluded")
	}
	if got["gogoanime"] != (struct {
		size int
		ff   bool
	}{5, false}) {
		t.Fatalf("gogoanime plan=%+v want {5,false}", got["gogoanime"])
	}
	if got["allanime"] != (struct {
		size int
		ff   bool
	}{1, true}) {
		t.Fatalf("allanime plan=%+v want {1,true}", got["allanime"])
	}
}
