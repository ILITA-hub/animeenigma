package scraperprovider_test

import (
	"os"
	"path/filepath"
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

func writeYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "providers.yaml")
	y := `providers:
  - name: allanime
    enabled: true
    supports_sub: true
    supports_dub: true
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 90
  - name: 18anime
    enabled: true
    group: adult
    supports_raw: true
    preference_weight: 0
`
	if err := os.WriteFile(p, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSeedFromYAML_InsertsRows(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedFromYAML(db, writeYAML(t)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 2 {
		t.Fatalf("want 2 rows, got %d", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" {
		t.Errorf("allanime seeded wrong: %+v", all)
	}
}

func TestSeedFromYAML_IdempotentDoesNotOverwrite(t *testing.T) {
	db := newDB(t)
	path := writeYAML(t)
	if err := scraperprovider.SeedFromYAML(db, path); err != nil {
		t.Fatalf("seed1: %v", err)
	}
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime").Update("enabled", false)
	if err := scraperprovider.SeedFromYAML(db, path); err != nil {
		t.Fatalf("seed2: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 2 {
		t.Fatalf("re-seed created duplicates: %d rows", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if all.Enabled {
		t.Error("re-seed overwrote operator edit (enabled flipped back to true)")
	}
}
