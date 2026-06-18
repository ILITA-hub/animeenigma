package scraperprovider_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEmitCatalogSideRoster_OnlyEmitsOwnedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// One catalog-owned row (scraper_operated=false) and one scraper-owned row
	// (scraper_operated=true). Catalog must emit ONLY the former.
	db.Create(&domain.ScraperProvider{Name: "t2_ae", Status: domain.StatusEnabled, Group: "firstparty", Reason: "first-party", Description: "self-hosted", ScraperOperated: false})
	db.Create(&domain.ScraperProvider{Name: "t2_gogo", Status: domain.StatusEnabled, Group: "en", ScraperOperated: true})

	if err := scraperprovider.EmitCatalogSideRoster(db); err != nil {
		t.Fatalf("emit: %v", err)
	}

	// Owned row reflected.
	if got := testutil.ToFloat64(metrics.ProviderEnabled.WithLabelValues("t2_ae")); got != 1 {
		t.Errorf("provider_enabled{t2_ae} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.ProviderInfo.WithLabelValues("t2_ae", "enabled", "first-party", "self-hosted")); got != 1 {
		t.Errorf("provider_info{t2_ae,...} = %v, want 1", got)
	}
	// Scraper-owned row must NOT be emitted by catalog (partition contract).
	// A never-Set() gauge reads 0, so this proves catalog skipped t2_gogo.
	if got := testutil.ToFloat64(metrics.ProviderEnabled.WithLabelValues("t2_gogo")); got != 0 {
		t.Errorf("provider_enabled{t2_gogo} = %v, want 0 — catalog must not emit scraper-owned rows", got)
	}
}
