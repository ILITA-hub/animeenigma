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
	if count != 8 {
		t.Fatalf("want 8 default rows, got %d", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" || !all.IsEnabled() {
		t.Errorf("allanime seeded wrong: %+v", all)
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
	if count != 8 {
		t.Fatalf("re-seed created duplicates: %d rows", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if all.Status != domain.StatusDisabled {
		t.Error("re-seed overwrote operator edit (status flipped back)")
	}
}
