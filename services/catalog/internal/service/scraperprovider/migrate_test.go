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
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "hanime").
		Update("status", domain.StatusDisabled).Error; err != nil {
		t.Fatalf("operator disable: %v", err)
	}
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

func TestMiruroDubOnly_FlipsMiruroSubOnly(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate a pre-existing live DB where miruro still advertised sub.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "miruro").
		Update("supports_sub", true).Error; err != nil {
		t.Fatalf("preset miruro supports_sub=true: %v", err)
	}

	if err := scraperprovider.MiruroDubOnly(db); err != nil {
		t.Fatalf("miruro dub-only: %v", err)
	}

	var miruro, gogo domain.ScraperProvider
	db.First(&miruro, "name = ?", "miruro")
	db.First(&gogo, "name = ?", "gogoanime")
	if miruro.SupportsSub {
		t.Error("miruro supports_sub should be false after MiruroDubOnly")
	}
	if !miruro.SupportsDub {
		t.Error("miruro supports_dub must stay true")
	}
	// Other providers untouched.
	if !gogo.SupportsSub {
		t.Error("gogoanime supports_sub must stay true (only miruro is flipped)")
	}
}

func TestMiruroDubOnly_GuardedDoesNotClobberOperatorReEnable(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.MiruroDubOnly(db); err != nil {
		t.Fatalf("miruro dub-only 1: %v", err)
	}
	// Operator later re-enables sub on miruro.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "miruro").
		Update("supports_sub", true).Error; err != nil {
		t.Fatalf("operator re-enable sub: %v", err)
	}
	// Second boot must NOT clobber the operator's re-enable (guard already set).
	if err := scraperprovider.MiruroDubOnly(db); err != nil {
		t.Fatalf("miruro dub-only 2: %v", err)
	}
	var miruro domain.ScraperProvider
	db.First(&miruro, "name = ?", "miruro")
	if !miruro.SupportsSub {
		t.Error("miruro supports_sub = false (guard clobbered operator re-enable)")
	}
}

func TestNineanimeBrowser_FlipsEngineAndBaseURL(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate a pre-existing live DB where nineanime was still engine='http'
	// (the seed shipped engine='browser' only for fresh DBs; an existing row is
	// never overwritten by the insert-if-absent seed).
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "nineanime").
		Updates(map[string]interface{}{"engine": "http", "base_url": ""}).Error; err != nil {
		t.Fatalf("preset nineanime engine=http: %v", err)
	}

	if err := scraperprovider.NineanimeBrowser(db); err != nil {
		t.Fatalf("nineanime browser: %v", err)
	}

	var nine, gogo domain.ScraperProvider
	db.First(&nine, "name = ?", "nineanime")
	db.First(&gogo, "name = ?", "gogoanime")
	if nine.Engine != "browser" {
		t.Errorf("nineanime engine = %q, want browser", nine.Engine)
	}
	if nine.BaseURL != "https://9anime.me.uk" {
		t.Errorf("nineanime base_url = %q, want https://9anime.me.uk", nine.BaseURL)
	}
	// Other providers untouched (gogoanime keeps its own browser base URL).
	if gogo.BaseURL != "https://gogoanimes.fi" {
		t.Errorf("gogoanime base_url = %q, must stay gogoanimes.fi (only nineanime is flipped)", gogo.BaseURL)
	}
}

func TestNineanimeBrowser_GuardedDoesNotClobberOperatorRevert(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.NineanimeBrowser(db); err != nil {
		t.Fatalf("nineanime browser 1: %v", err)
	}
	// Operator later reverts nineanime back to engine='http' in the DB.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "nineanime").
		Update("engine", "http").Error; err != nil {
		t.Fatalf("operator revert engine=http: %v", err)
	}
	// Second boot must NOT clobber the operator's revert (guard already set).
	if err := scraperprovider.NineanimeBrowser(db); err != nil {
		t.Fatalf("nineanime browser 2: %v", err)
	}
	var nine domain.ScraperProvider
	db.First(&nine, "name = ?", "nineanime")
	if nine.Engine != "http" {
		t.Errorf("nineanime engine = %q, want http (guard clobbered operator revert)", nine.Engine)
	}
}

func TestNineanimeBrowser_NoRow_ErrorsAndDoesNotWriteGuard(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// No nineanime row at all (seed did not run / row hard-deleted).
	if err := scraperprovider.NineanimeBrowser(db); err == nil {
		t.Fatal("NineanimeBrowser should error when no nineanime row exists")
	}
	// Guard must NOT be written, so a later boot (once the row exists) retries.
	var guards int64
	db.Table("catalog_migration_guards").Where("key = ?", "nineanime_browser").Count(&guards)
	if guards != 0 {
		t.Errorf("guard count = %d, want 0 (must not write guard on no-row failure)", guards)
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

func TestSplitKodik_RenamesLegacyKodikAndSeedsNoads(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Simulate a pre-split live DB: a single legacy "kodik" row.
	if err := db.Create(&domain.ScraperProvider{
		Name: "kodik", Status: domain.StatusEnabled, Group: "ru",
	}).Error; err != nil {
		t.Fatalf("seed legacy kodik: %v", err)
	}

	// SplitKodik (rename) must run BEFORE SeedDefaults, mirroring main.go order.
	if err := scraperprovider.SplitKodik(db); err != nil {
		t.Fatalf("split kodik: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var legacy int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "kodik").Count(&legacy)
	if legacy != 0 {
		t.Errorf("legacy 'kodik' row must be gone after split, found %d", legacy)
	}
	var iframe, noads domain.ScraperProvider
	if err := db.First(&iframe, "name = ?", "kodik-iframe").Error; err != nil {
		t.Fatalf("kodik-iframe row missing: %v", err)
	}
	if iframe.Group != "ru" {
		t.Errorf("kodik-iframe group = %q, want ru", iframe.Group)
	}
	if err := db.First(&noads, "name = ?", "kodik-noads").Error; err != nil {
		t.Fatalf("kodik-noads row missing: %v", err)
	}
	if noads.Group != "ru" || noads.Status != domain.StatusEnabled {
		t.Errorf("kodik-noads = %+v, want group ru / enabled", noads)
	}
}

func TestSplitKodik_FreshDB_NoLegacyRow(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Fresh DB: no kodik row at all. SplitKodik then SeedDefaults.
	if err := scraperprovider.SplitKodik(db); err != nil {
		t.Fatalf("split kodik (fresh): %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, name := range []string{"kodik-iframe", "kodik-noads"} {
		var c int64
		db.Model(&domain.ScraperProvider{}).Where("name = ?", name).Count(&c)
		if c != 1 {
			t.Errorf("%s count = %d, want 1 on fresh DB", name, c)
		}
	}
	var legacy int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "kodik").Count(&legacy)
	if legacy != 0 {
		t.Errorf("no legacy 'kodik' row should exist on fresh DB, found %d", legacy)
	}
}

func TestSplitKodik_Idempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.AutoMigrate(&domain.ScraperProvider{})
	_ = db.Create(&domain.ScraperProvider{Name: "kodik", Status: domain.StatusEnabled, Group: "ru"}).Error
	if err := scraperprovider.SplitKodik(db); err != nil {
		t.Fatalf("split 1: %v", err)
	}
	// Re-running must be a guarded no-op (no error, no duplicate).
	if err := scraperprovider.SplitKodik(db); err != nil {
		t.Fatalf("split 2 (idempotent): %v", err)
	}
	var iframe int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "kodik-iframe").Count(&iframe)
	if iframe != 1 {
		t.Errorf("kodik-iframe count = %d, want 1 after double-run", iframe)
	}
}
