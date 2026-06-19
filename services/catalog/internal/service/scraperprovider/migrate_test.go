package scraperprovider_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRenameScraperProvidersTable_RenamesLegacy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Simulate a legacy DB: a physical table literally named scraper_providers.
	if err := db.Exec(`CREATE TABLE scraper_providers (name text primary key, status text)`).Error; err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := db.Exec(`INSERT INTO scraper_providers(name,status) VALUES ('gogoanime','enabled')`).Error; err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	if err := scraperprovider.RenameScraperProvidersTable(db); err != nil {
		t.Fatalf("rename: %v", err)
	}

	m := db.Migrator()
	if m.HasTable("scraper_providers") {
		t.Error("old scraper_providers table still exists after rename")
	}
	if !m.HasTable("stream_providers") {
		t.Fatal("stream_providers table missing after rename")
	}
	var name string
	if err := db.Raw(`SELECT name FROM stream_providers WHERE name='gogoanime'`).Scan(&name).Error; err != nil || name != "gogoanime" {
		t.Errorf("row not preserved through rename: name=%q err=%v", name, err)
	}
}

func TestRenameScraperProvidersTable_IdempotentOnFreshDB(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	// Fresh DB: only the new table exists (AutoMigrate would have made it).
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.RenameScraperProvidersTable(db); err != nil {
		t.Fatalf("rename on fresh DB should be a no-op, got: %v", err)
	}
	if !db.Migrator().HasTable("stream_providers") {
		t.Error("stream_providers missing after no-op rename")
	}
}

func TestRetireHanimeAnimelib_DisablesExactlyThoseTwo(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := scraperprovider.RetireHanimeAnimelib(db); err != nil {
		t.Fatalf("retire: %v", err)
	}

	var hanime, animelib, kodik domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	db.First(&animelib, "name = ?", "animelib")
	db.First(&kodik, "name = ?", "kodik")

	if hanime.Status != domain.StatusDisabled {
		t.Errorf("hanime status = %q, want disabled", hanime.Status)
	}
	if animelib.Status != domain.StatusDisabled {
		t.Errorf("animelib status = %q, want disabled", animelib.Status)
	}
	// Control: a sibling legacy row must NOT be touched.
	if kodik.Status == domain.StatusDisabled {
		t.Errorf("kodik status = %q, must NOT be disabled by RetireHanimeAnimelib", kodik.Status)
	}
}

func TestRetireHanimeAnimelib_GuardedDoesNotClobberReEnable(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.RetireHanimeAnimelib(db); err != nil {
		t.Fatalf("retire1: %v", err)
	}
	// Operator manually re-enables hanime later.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "hanime").
		Update("status", domain.StatusEnabled)

	// Second boot: the guarded migration must NOT clobber the operator's re-enable.
	if err := scraperprovider.RetireHanimeAnimelib(db); err != nil {
		t.Fatalf("retire2: %v", err)
	}
	var hanime domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	if hanime.Status != domain.StatusEnabled {
		t.Errorf("hanime status = %q, want enabled (guarded migration clobbered operator re-enable)", hanime.Status)
	}
}

func TestReEnableHanime_EnablesHanimeOnly(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate the live-DB state: Plan B retired hanime + animelib.
	if err := scraperprovider.RetireHanimeAnimelib(db); err != nil {
		t.Fatalf("retire: %v", err)
	}

	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable: %v", err)
	}

	var hanime, animelib domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	db.First(&animelib, "name = ?", "animelib")
	if hanime.Status != domain.StatusEnabled {
		t.Errorf("hanime status = %q, want enabled", hanime.Status)
	}
	// animelib must stay retired — only hanime is restored.
	if animelib.Status != domain.StatusDisabled {
		t.Errorf("animelib status = %q, want disabled", animelib.Status)
	}
}

func TestReEnableHanime_GuardedDoesNotClobberOperatorDisable(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable1: %v", err)
	}
	// Operator later disables hanime again.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "hanime").
		Update("status", domain.StatusDisabled)
	// Second boot must NOT clobber the operator's disable (guard already set).
	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable2: %v", err)
	}
	var hanime domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	if hanime.Status != domain.StatusDisabled {
		t.Errorf("hanime status = %q, want disabled (guard clobbered operator disable)", hanime.Status)
	}
}

func TestBackfillScraperOperated_SetsIntrinsicFlag(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// One scraper provider, one first-party — both default scraper_operated=false.
	db.Create(&domain.ScraperProvider{Name: "gogoanime", Status: domain.StatusEnabled})
	db.Create(&domain.ScraperProvider{Name: "ae", Status: domain.StatusEnabled})

	if err := scraperprovider.BackfillScraperOperated(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var gogo, ae domain.ScraperProvider
	db.First(&gogo, "name = ?", "gogoanime")
	db.First(&ae, "name = ?", "ae")
	if !gogo.ScraperOperated {
		t.Error("gogoanime should be scraper_operated=true")
	}
	if ae.ScraperOperated {
		t.Error("ae should be scraper_operated=false")
	}
}
