package scraperprovider_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSeedDefaults_InsertsRoster(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 17 {
		t.Fatalf("want 17 default rows, got %d", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" || !all.IsDegraded() {
		t.Errorf("allanime seeded wrong: %+v", all)
	}
	if !all.ScraperOperated {
		t.Error("allanime should be scraper_operated=true")
	}
	var ae domain.ScraperProvider
	db.First(&ae, "name = ?", "ae")
	if ae.Group != "firstparty" || !ae.IsEnabled() || ae.ScraperOperated {
		t.Errorf("ae seeded wrong (want firstparty/enabled/not-scraper-operated): %+v", ae)
	}
	var kodikIframe domain.ScraperProvider
	db.First(&kodikIframe, "name = ?", "kodik-iframe")
	if kodikIframe.Group != "ru" || kodikIframe.ScraperOperated {
		t.Errorf("kodik-iframe seeded wrong (want ru/not-scraper-operated): %+v", kodikIframe)
	}
	var kodikNoads domain.ScraperProvider
	db.First(&kodikNoads, "name = ?", "kodik-noads")
	if kodikNoads.Group != "ru" || kodikNoads.ScraperOperated || !kodikNoads.IsEnabled() {
		t.Errorf("kodik-noads seeded wrong (want ru/enabled/not-scraper-operated): %+v", kodikNoads)
	}
	var okru domain.ScraperProvider
	db.First(&okru, "name = ?", "okru")
	if !okru.IsEnabled() || okru.Group != "en" || !okru.ScraperOperated {
		t.Errorf("okru seeded wrong (want en/enabled/scraper-operated): %+v", okru)
	}
	// sub_delivery "unknown": claimed hard but unverified by the 2026-06-29 subprobe (stream down).
	if !okru.SupportsSub || !okru.SupportsDub || okru.SubDelivery != "unknown" || okru.PreferenceWeight != 35 {
		t.Errorf("okru capabilities wrong (want sub+dub/unknown/35): %+v", okru)
	}
	// The two animejoy RU-sub legs: intrinsic group ru, NOT scraper-operated (kept
	// out of the EN failover chain), enabled (promoted out of soak 2026-06-30 —
	// probe-verified — so they surface for all users), sub-only/hard.
	for _, tc := range []struct {
		name   string
		weight int
	}{
		{"animejoy-sibnet", 25},
		{"animejoy-allvideo", 20},
	} {
		var aj domain.ScraperProvider
		if err := db.First(&aj, "name = ?", tc.name).Error; err != nil {
			t.Fatalf("%s row missing: %v", tc.name, err)
		}
		if aj.Group != "ru" || aj.ScraperOperated || !aj.IsEnabled() {
			t.Errorf("%s seeded wrong (want ru/enabled/not-scraper-operated): %+v", tc.name, aj)
		}
		if !aj.SupportsSub || aj.SupportsDub || aj.SupportsRaw {
			t.Errorf("%s capabilities wrong (want sub-only): %+v", tc.name, aj)
		}
		if aj.SubDelivery != "hard" || aj.QualityCeiling != "1080p" || aj.PreferenceWeight != tc.weight {
			t.Errorf("%s traits wrong (want hard/1080p/%d): %+v", tc.name, tc.weight, aj)
		}
	}
}

func TestSeedDefaults_AnimeFeverDegradedWithReason(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var af domain.ScraperProvider
	if err := db.First(&af, "name = ?", "animefever").Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if !af.IsDegraded() {
		t.Errorf("animefever status = %q, want degraded", af.Status)
	}
	if af.Reason == "" || af.Description == "" {
		t.Errorf("animefever must carry a reason/description in the DB: %+v", af)
	}
}

func TestSeedDefaults_IntrinsicGroupForAdult(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var row domain.ScraperProvider
	if err := db.First(&row, "name = ?", "18anime").Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if row.Group != "adult" {
		t.Errorf("18anime group = %q, want adult (intrinsic)", row.Group)
	}
}

func TestSeedDefaults_IdempotentDoesNotOverwrite(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed1: %v", err)
	}
	// Operator flips allanime to disabled in the DB.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime").
		Update("status", domain.StatusDisabled)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed2: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 17 {
		t.Fatalf("re-seed created duplicates: %d rows", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if all.Status != domain.StatusDisabled {
		t.Error("re-seed overwrote operator edit (status flipped back)")
	}
}
