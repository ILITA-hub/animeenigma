package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testNopLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// TestList_DerivesWireStatus verifies that the List handler emits status derived
// via WireStatus() (from policy+health) rather than the stored Status column,
// and that policy/health appear in the wire output.
func TestList_DerivesWireStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	// gogoanime: policy=auto + health=up → WireStatus()=enabled
	// allanime:  policy=manual + health=recovering → WireStatus()=degraded
	// Both have Status="" (zero) in DB — proves we derive, not read the column.
	if err := db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, ScraperOperated: true},
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthRecovering, ScraperOperated: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	h := handler.NewInternalScraperProvidersHandler(db, testNopLogger())
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest(http.MethodGet, "/internal/scraper/providers", nil))

	var body struct {
		Data struct {
			Providers []map[string]any `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}
	got := map[string]map[string]any{}
	for _, p := range body.Data.Providers {
		got[p["name"].(string)] = p
	}
	if got["gogoanime"]["status"] != "enabled" {
		t.Fatalf("gogoanime status=%v want enabled", got["gogoanime"]["status"])
	}
	if got["allanime"]["status"] != "degraded" {
		t.Fatalf("allanime status=%v want degraded", got["allanime"]["status"])
	}
	if got["allanime"]["health"] != "recovering" || got["allanime"]["policy"] != "manual" {
		t.Fatalf("allanime missing policy/health: %+v", got["allanime"])
	}
}

func TestInternalScraperProviders_List(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "nineanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 40}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", PreferenceWeight: 90}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalScraperProvidersHandler(db, testNopLogger())
	req := httptest.NewRequest(http.MethodGet, "/internal/scraper/providers", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	// httputil.OK wraps in {success, data: {...}} — see libs/httputil/response.go:128.
	var body struct {
		Data struct {
			Providers []domain.ScraperProvider `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(body.Data.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d", len(body.Data.Providers))
	}
	if body.Data.Providers[0].Name != "allanime" || body.Data.Providers[1].Name != "nineanime" {
		t.Errorf("order = %s,%s want allanime,nineanime", body.Data.Providers[0].Name, body.Data.Providers[1].Name)
	}
}
