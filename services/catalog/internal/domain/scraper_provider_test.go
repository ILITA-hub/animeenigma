package domain_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScraperProviderSchema_AutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if !db.Migrator().HasTable("scraper_providers") {
		t.Fatal("scraper_providers table not created")
	}
	for _, col := range []string{"name", "enabled", "group", "supports_sub", "supports_dub", "supports_raw", "sub_delivery", "quality_ceiling", "preference_weight"} {
		if !db.Migrator().HasColumn(&domain.ScraperProvider{}, col) {
			t.Errorf("missing column %q", col)
		}
	}
	row := domain.ScraperProvider{Name: "allanime", Enabled: true, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 90}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got domain.ScraperProvider
	if err := db.First(&got, "name = ?", "allanime").Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.PreferenceWeight != 90 || got.SubDelivery != "hard" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
