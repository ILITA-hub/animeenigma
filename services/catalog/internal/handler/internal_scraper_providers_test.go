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

func TestInternalScraperProviders_List(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "nineanime", Enabled: true, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 40}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "allanime", Enabled: true, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", PreferenceWeight: 90}).Error; err != nil {
		t.Fatal(err)
	}

	nopLog := &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
	h := handler.NewInternalScraperProvidersHandler(db, nopLog)
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
