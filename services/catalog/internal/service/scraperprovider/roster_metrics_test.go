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

func TestEmitProviderStates_CarriesGroupLabel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&[]domain.ScraperProvider{
		{Name: "t2_gogoanime", Group: "en", Policy: domain.PolicyAuto, Health: domain.HealthUp},
		// Parked-but-alive: manual + confirmed Down.
		{Name: "t2_parked", Group: "en", Policy: domain.PolicyManual, Health: domain.HealthDown},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.EmitProviderStates(db); err != nil {
		t.Fatalf("emit: %v", err)
	}
	// auto+up: both gauges carry 4 on the (provider, group) series.
	if got := testutil.ToFloat64(metrics.ProviderState.WithLabelValues("t2_gogoanime", "en")); got != 4 {
		t.Errorf("provider_state{t2_gogoanime,en} = %v, want 4", got)
	}
	if got := testutil.ToFloat64(metrics.ProviderHealthState.WithLabelValues("t2_gogoanime", "en")); got != 4 {
		t.Errorf("provider_health_state{t2_gogoanime,en} = %v, want 4", got)
	}
	// Parked manual+down: the failover-participation gauge collapses to 0 (out of
	// auto-failover, so the fleet alerts ignore it), but the health-timeline gauge
	// shows its LIVE Down health (1).
	if got := testutil.ToFloat64(metrics.ProviderState.WithLabelValues("t2_parked", "en")); got != 0 {
		t.Errorf("provider_state{t2_parked,en} = %v, want 0 (manual is out of auto-failover)", got)
	}
	if got := testutil.ToFloat64(metrics.ProviderHealthState.WithLabelValues("t2_parked", "en")); got != 1 {
		t.Errorf("provider_health_state{t2_parked,en} = %v, want 1 (Down health shown)", got)
	}
}
