package scraperprovider_test

import (
	"strings"
	"testing"
	"time"

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

func TestRemoveRawProvider_DeletesRowIdempotently(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Simulate a pre-existing live DB that still has the legacy raw row (the seed
	// no longer creates it).
	if err := db.Create(&domain.ScraperProvider{Name: "raw", Status: domain.StatusEnabled, Group: "jp"}).Error; err != nil {
		t.Fatalf("preset raw row: %v", err)
	}
	// A sibling row that must survive.
	if err := db.Create(&domain.ScraperProvider{Name: "ae", Status: domain.StatusEnabled, Group: "firstparty"}).Error; err != nil {
		t.Fatalf("preset ae row: %v", err)
	}

	if err := scraperprovider.RemoveRawProvider(db); err != nil {
		t.Fatalf("remove raw provider: %v", err)
	}
	var rawCount, aeCount int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "raw").Count(&rawCount)
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "ae").Count(&aeCount)
	if rawCount != 0 {
		t.Errorf("raw row count = %d, want 0 after RemoveRawProvider", rawCount)
	}
	if aeCount != 1 {
		t.Errorf("ae row must survive: count = %d, want 1", aeCount)
	}
	// Idempotent: a second call (and a fresh-DB no-row case) is a clean no-op.
	if err := scraperprovider.RemoveRawProvider(db); err != nil {
		t.Fatalf("second RemoveRawProvider call must be a no-op: %v", err)
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

func TestAnimepaheBrowserRevival_FlipsAndPromotesToDegraded(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate a pre-existing LIVE DB where animepahe is fully disabled: the
	// retirement migration + policy/health backfill already ran, leaving it
	// engine=http, policy=disabled, health=down. WireStatus() is therefore
	// StatusDisabled — flipping `status` alone would NOT register it.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "animepahe").
		Updates(map[string]interface{}{
			"engine": "http", "base_url": "",
			"status": string(domain.StatusDisabled),
			"policy": string(domain.PolicyDisabled), "health": string(domain.HealthDown),
		}).Error; err != nil {
		t.Fatalf("preset animepahe disabled: %v", err)
	}

	if err := scraperprovider.AnimepaheBrowserRevival(db); err != nil {
		t.Fatalf("animepahe browser revival: %v", err)
	}

	var pahe, gogo domain.ScraperProvider
	db.First(&pahe, "name = ?", "animepahe")
	db.First(&gogo, "name = ?", "gogoanime")
	if pahe.Engine != "browser" {
		t.Errorf("animepahe engine = %q, want browser", pahe.Engine)
	}
	if pahe.BaseURL != "https://animepahe.pw" {
		t.Errorf("animepahe base_url = %q, want https://animepahe.pw", pahe.BaseURL)
	}
	if pahe.Policy != domain.PolicyManual {
		t.Errorf("animepahe policy = %q, want manual", pahe.Policy)
	}
	// The whole point: the DERIVED wire status must be degraded (registered +
	// manually selectable), NOT disabled.
	if got := pahe.WireStatus(); got != domain.StatusDegraded {
		t.Errorf("animepahe WireStatus() = %q, want degraded", got)
	}
	// Other providers untouched.
	if gogo.BaseURL != "https://gogoanimes.fi" {
		t.Errorf("gogoanime base_url = %q, must stay gogoanimes.fi (only animepahe flipped)", gogo.BaseURL)
	}
}

func TestAnimepaheBrowserRevival_GuardedDoesNotClobberOperatorRevert(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.AnimepaheBrowserRevival(db); err != nil {
		t.Fatalf("animepahe browser revival 1: %v", err)
	}
	// Operator later disables animepahe again in the DB.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "animepahe").
		Update("policy", string(domain.PolicyDisabled)).Error; err != nil {
		t.Fatalf("operator disable: %v", err)
	}
	// Second boot must NOT clobber the operator's change (guard already set).
	if err := scraperprovider.AnimepaheBrowserRevival(db); err != nil {
		t.Fatalf("animepahe browser revival 2: %v", err)
	}
	var pahe domain.ScraperProvider
	db.First(&pahe, "name = ?", "animepahe")
	if pahe.Policy != domain.PolicyDisabled {
		t.Errorf("animepahe policy = %q, want disabled (guard clobbered operator change)", pahe.Policy)
	}
}

func TestMiruroBrowserRevival_FlipsToBrowserAndDegraded(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate the pre-block LIVE DB: miruro was engine=http, auto/up (a healthy
	// auto-failover provider before the 2026-07-02 Cloudflare block).
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "miruro").
		Updates(map[string]interface{}{
			"engine": "http", "base_url": "",
			"status": string(domain.StatusEnabled),
			"policy": string(domain.PolicyAuto), "health": string(domain.HealthUp),
		}).Error; err != nil {
		t.Fatalf("preset miruro pre-block: %v", err)
	}

	if err := scraperprovider.MiruroBrowserRevival(db); err != nil {
		t.Fatalf("miruro browser revival: %v", err)
	}

	var mir, gogo domain.ScraperProvider
	db.First(&mir, "name = ?", "miruro")
	db.First(&gogo, "name = ?", "gogoanime")
	if mir.Engine != "browser" {
		t.Errorf("miruro engine = %q, want browser", mir.Engine)
	}
	if mir.BaseURL != "https://www.miruro.tv" {
		t.Errorf("miruro base_url = %q, want https://www.miruro.tv", mir.BaseURL)
	}
	if mir.Policy != domain.PolicyManual {
		t.Errorf("miruro policy = %q, want manual", mir.Policy)
	}
	// The whole point: DERIVED wire status must be degraded (registered + manually
	// selectable), NOT enabled (auto-failover) and NOT disabled.
	if got := mir.WireStatus(); got != domain.StatusDegraded {
		t.Errorf("miruro WireStatus() = %q, want degraded", got)
	}
	// Other providers untouched.
	if gogo.BaseURL != "https://gogoanimes.fi" {
		t.Errorf("gogoanime base_url = %q, must stay gogoanimes.fi (only miruro flipped)", gogo.BaseURL)
	}
}

func TestMiruroBrowserRevival_GuardedDoesNotClobberOperatorRevert(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.MiruroBrowserRevival(db); err != nil {
		t.Fatalf("miruro browser revival 1: %v", err)
	}
	// Operator later flips miruro back to engine=http in the DB.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "miruro").
		Update("engine", "http").Error; err != nil {
		t.Fatalf("operator revert: %v", err)
	}
	// Second boot must NOT clobber the operator's change (guard already set).
	if err := scraperprovider.MiruroBrowserRevival(db); err != nil {
		t.Fatalf("miruro browser revival 2: %v", err)
	}
	var mir domain.ScraperProvider
	db.First(&mir, "name = ?", "miruro")
	if mir.Engine != "http" {
		t.Errorf("miruro engine = %q, want http (guard clobbered operator revert)", mir.Engine)
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

func TestAnimefeverDisable_FlipsDegradedToDisabledOnceIdempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-change live-DB shape: an animefever row still on degraded (the seed
	// default before this change; AnimefeverDeclaim only refreshed reason/desc).
	if err := db.Create(&domain.ScraperProvider{Name: "animefever", Status: domain.StatusDegraded}).Error; err != nil {
		t.Fatal(err)
	}
	if err := scraperprovider.AnimefeverDisable(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	var row domain.ScraperProvider
	db.Where("name = ?", "animefever").First(&row)
	if row.Status != domain.StatusDisabled {
		t.Fatalf("status = %q, want disabled", row.Status)
	}
	// operator re-enables; second run must NOT clobber (guard already written)
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "animefever").Update("status", domain.StatusEnabled)
	if err := scraperprovider.AnimefeverDisable(db); err != nil {
		t.Fatalf("second: %v", err)
	}
	db.Where("name = ?", "animefever").First(&row)
	if row.Status != domain.StatusEnabled {
		t.Fatalf("status = %q after re-enable+rerun, want enabled (not clobbered)", row.Status)
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

func TestBackfillPolicyHealth(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seed := []domain.ScraperProvider{
		{Name: "gogoanime", Status: domain.StatusEnabled, UpdatedAt: now},
		{Name: "allanime", Status: domain.StatusDegraded, UpdatedAt: now},
		{Name: "deadguy", Status: domain.StatusDisabled, UpdatedAt: now},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.BackfillPolicyHealth(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	want := map[string][2]string{
		"gogoanime": {"auto", "up"},
		"allanime":  {"manual", "down"},
		"deadguy":   {"disabled", "down"},
	}
	for name, exp := range want {
		var p domain.ScraperProvider
		db.First(&p, "name = ?", name)
		if string(p.Policy) != exp[0] || string(p.Health) != exp[1] {
			t.Fatalf("%s: got (%s,%s) want (%s,%s)", name, p.Policy, p.Health, exp[0], exp[1])
		}
		if p.HealthSince.IsZero() || p.PolicySince.IsZero() {
			t.Fatalf("%s: timestamps not set", name)
		}
	}

	// Idempotent: second run is a no-op (guard).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "gogoanime").Update("policy", "manual")
	if err := scraperprovider.BackfillPolicyHealth(db); err != nil {
		t.Fatal(err)
	}
	var g domain.ScraperProvider
	db.First(&g, "name = ?", "gogoanime")
	if g.Policy != "manual" {
		t.Fatalf("guard failed: backfill re-ran and clobbered operator edit")
	}
}

func TestAddAnimejoyProviders_InsertsBothRowsExplicitGroupAndFlag(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Simulate an existing live DB that pre-dates animejoy (no rows present, and
	// the policy/health backfill has NOT been re-run for these names).
	if err := scraperprovider.AddAnimejoyProviders(db); err != nil {
		t.Fatalf("add animejoy: %v", err)
	}

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
		// CRITICAL: raw migration must set group='ru' + scraper_operated=false
		// EXPLICITLY (intrinsicGroup stamping does not run here).
		if aj.Group != "ru" {
			t.Errorf("%s group = %q, want ru (must be set explicitly in raw migration)", tc.name, aj.Group)
		}
		if aj.ScraperOperated {
			t.Errorf("%s scraper_operated = true, want false (must stay out of EN failover chain)", tc.name)
		}
		if aj.Status != domain.StatusDegraded {
			t.Errorf("%s status = %q, want degraded", tc.name, aj.Status)
		}
		// policy=manual + health=down → WireStatus() degraded regardless of
		// BackfillPolicyHealth ordering.
		if aj.Policy != domain.PolicyManual || aj.Health != domain.HealthDown {
			t.Errorf("%s (policy,health) = (%q,%q), want (manual,down)", tc.name, aj.Policy, aj.Health)
		}
		if got := aj.WireStatus(); got != domain.StatusDegraded {
			t.Errorf("%s WireStatus() = %q, want degraded", tc.name, got)
		}
		if !aj.SupportsSub || aj.SupportsDub || aj.SupportsRaw || aj.SubDelivery != "hard" || aj.PreferenceWeight != tc.weight {
			t.Errorf("%s traits wrong (want sub-only/hard/%d): %+v", tc.name, tc.weight, aj)
		}
	}
}

func TestAddAnimejoyProviders_IdempotentAndDoesNotClobber(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.AddAnimejoyProviders(db); err != nil {
		t.Fatalf("add animejoy 1: %v", err)
	}
	// Operator later promotes one leg in the DB.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "animejoy-sibnet").
		Update("status", domain.StatusEnabled).Error; err != nil {
		t.Fatalf("operator promote: %v", err)
	}
	// Second boot: guard already set → must be a no-op (no clobber, no duplicate).
	if err := scraperprovider.AddAnimejoyProviders(db); err != nil {
		t.Fatalf("add animejoy 2: %v", err)
	}

	var count int64
	db.Model(&domain.ScraperProvider{}).Where("name LIKE ?", "animejoy-%").Count(&count)
	if count != 2 {
		t.Fatalf("animejoy row count = %d, want 2 (no duplicates)", count)
	}
	var sib domain.ScraperProvider
	db.First(&sib, "name = ?", "animejoy-sibnet")
	if sib.Status != domain.StatusEnabled {
		t.Errorf("animejoy-sibnet status = %q, want enabled (guard clobbered operator promote)", sib.Status)
	}
}

func TestAddAnimejoyProviders_InsertIfAbsentSkipsExistingRow(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Fresh DB seeded first (the rows already exist via SeedDefaults), THEN the
	// migration runs on the same DB — it must not duplicate or overwrite.
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Operator edits an existing seeded row before the migration runs.
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "animejoy-allvideo").
		Update("preference_weight", 99).Error; err != nil {
		t.Fatalf("operator edit: %v", err)
	}
	if err := scraperprovider.AddAnimejoyProviders(db); err != nil {
		t.Fatalf("add animejoy: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Where("name LIKE ?", "animejoy-%").Count(&count)
	if count != 2 {
		t.Fatalf("animejoy row count = %d, want 2 (insert-if-absent must not duplicate seeded rows)", count)
	}
	var av domain.ScraperProvider
	db.First(&av, "name = ?", "animejoy-allvideo")
	if av.PreferenceWeight != 99 {
		t.Errorf("animejoy-allvideo preference_weight = %d, want 99 (insert-if-absent clobbered operator edit)", av.PreferenceWeight)
	}
}

func TestAnimepaheSidecarRetired_RefreshesRecordOnceIdempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-existing live-DB state: animepahe disabled with the OLD reason that
	// still cites the now-deleted sidecar.
	if err := db.Create(&domain.ScraperProvider{
		Name: "animepahe", Status: domain.StatusDisabled, Reason: "Cloudflare challenge",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.AnimepaheSidecarRetired(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	var row domain.ScraperProvider
	db.Where("name = ?", "animepahe").First(&row)
	if row.Status != domain.StatusDisabled {
		t.Fatalf("status = %q, want disabled", row.Status)
	}
	if row.Reason != "Off — animepahe-resolver sidecar retired (2026-06-24)" {
		t.Fatalf("reason not refreshed: %q", row.Reason)
	}
	if !strings.Contains(row.Description, "retired 2026-06-24") || !strings.Contains(row.Description, "can be revived") {
		t.Fatalf("description not refreshed to the retired+revivable story: %q", row.Description)
	}

	// Operator revives animepahe; a second run must NOT clobber it (guard written).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "animepahe").Update("status", domain.StatusEnabled)
	if err := scraperprovider.AnimepaheSidecarRetired(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	db.Where("name = ?", "animepahe").First(&row)
	if row.Status != domain.StatusEnabled {
		t.Fatalf("status = %q after operator re-enable + rerun, want enabled (not clobbered)", row.Status)
	}
}

func TestMiruroCloudflareBlock_RefreshesDescriptionOnceIdempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-existing live-DB state: miruro auto+down with the probe-managed generic
	// reason and the old (pre-CF-block) description.
	if err := db.Create(&domain.ScraperProvider{
		Name: "miruro", Status: domain.StatusDegraded, Policy: domain.PolicyAuto, Health: domain.HealthDown,
		Reason:      "cdn_unreachable on ",
		Description: "Miruro aggregator: AnimePahe/kwik.cx HLS via the kiwi server (vault-*.uwucdn HLS), 1080p, AES-128 encrypted segments served through the streaming proxy. EN sub. Playback-probed.",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.MiruroCloudflareBlock(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	var row domain.ScraperProvider
	db.Where("name = ?", "miruro").First(&row)
	if !strings.Contains(row.Description, "Cloudflare WAF managed-rule block") {
		t.Fatalf("description not refreshed with the CF-block finding: %q", row.Description)
	}
	// reason/status/policy/health are NOT this migration's concern — untouched.
	if row.Reason != "cdn_unreachable on " {
		t.Fatalf("reason should be untouched (probe-managed): %q", row.Reason)
	}
	if row.Policy != domain.PolicyAuto || row.Health != domain.HealthDown {
		t.Fatalf("policy/health should be untouched (state-machine-owned): policy=%q health=%q", row.Policy, row.Health)
	}

	// A later operator's own description edit must NOT be clobbered by a rerun
	// (guard already written).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "miruro").Update("description", "operator note")
	if err := scraperprovider.MiruroCloudflareBlock(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	db.Where("name = ?", "miruro").First(&row)
	if row.Description != "operator note" {
		t.Fatalf("description = %q after operator edit + rerun, want untouched (not clobbered)", row.Description)
	}
}

func TestAllanimeOkruMerge_RenamesOkruAndTombstonesAllanime(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-existing live-DB state: okru enabled, allanime degraded (the shape
	// before the fold).
	if err := db.Create(&domain.ScraperProvider{Name: "okru", Status: domain.StatusEnabled, PreferenceWeight: 35}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusDegraded, PreferenceWeight: 90}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.AllanimeOkruMerge(db); err != nil {
		t.Fatalf("AllanimeOkruMerge: %v", err)
	}

	var okru int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "okru").Count(&okru)
	if okru != 0 {
		t.Errorf("okru row still present, want renamed")
	}
	var merged domain.ScraperProvider
	if err := db.Where("name = ?", "allanime-okru").First(&merged).Error; err != nil {
		t.Fatalf("allanime-okru row missing: %v", err)
	}
	if merged.Status != domain.StatusEnabled || merged.PreferenceWeight != 35 {
		t.Errorf("allanime-okru = %v/%d, want enabled/35", merged.Status, merged.PreferenceWeight)
	}
	var old domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&old)
	if old.Status != domain.StatusDisabled {
		t.Errorf("allanime status = %v, want disabled", old.Status)
	}

	// Idempotent: second run does not error and changes nothing.
	if err := scraperprovider.AllanimeOkruMerge(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

// TestAllanimeOkruMerge_BeforeSeed_PreservesLegacyWeight reproduces the REAL boot
// order (the ordering bug fix in main.go): AllanimeOkruMerge MUST run BEFORE
// SeedDefaults. If it instead ran after, SeedDefaults' insert-if-absent would
// already have created a fresh allanime-okru row (weight 35) by the time the
// merge's rename executed, its `already==0` guard would be false, the rename
// would be skipped, and the legacy okru row (with its tuned weight) would be
// orphaned forever. Calling merge first proves the rename wins and the legacy
// weight survives the seed running afterward.
func TestAllanimeOkruMerge_BeforeSeed_PreservesLegacyWeight(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Legacy live-DB shape: a tuned, enabled okru row and a degraded allanime
	// row, with NO allanime-okru row yet.
	if err := db.Create(&domain.ScraperProvider{
		Name: "okru", Status: domain.StatusEnabled, PreferenceWeight: 77,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name: "allanime", Status: domain.StatusDegraded,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Fixed boot order: merge BEFORE seed.
	if err := scraperprovider.AllanimeOkruMerge(db); err != nil {
		t.Fatalf("AllanimeOkruMerge: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	var okruCount int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "okru").Count(&okruCount)
	if okruCount != 0 {
		t.Errorf("legacy okru row count = %d, want 0 (must be renamed away, not orphaned)", okruCount)
	}

	var mergedCount int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime-okru").Count(&mergedCount)
	if mergedCount != 1 {
		t.Fatalf("allanime-okru row count = %d, want exactly 1 (no duplicate from the seed)", mergedCount)
	}
	var merged domain.ScraperProvider
	db.Where("name = ?", "allanime-okru").First(&merged)
	if merged.PreferenceWeight != 77 {
		t.Errorf("allanime-okru preference_weight = %d, want 77 (legacy tuned weight must survive, not reset to the seed's 35)", merged.PreferenceWeight)
	}
	if merged.Status != domain.StatusEnabled {
		t.Errorf("allanime-okru status = %q, want enabled", merged.Status)
	}

	var old domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&old)
	if old.Status != domain.StatusDisabled {
		t.Errorf("allanime status = %q, want disabled", old.Status)
	}
}

func TestAllanimeOkruMerge_FreshDBSeedAlreadyCorrect_NoopNotError(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Fresh-DB shape: the seed already wrote allanime-okru directly (no okru
	// row exists) and allanime as disabled.
	if err := db.Create(&domain.ScraperProvider{Name: "allanime-okru", Status: domain.StatusEnabled, PreferenceWeight: 35}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusDisabled}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.AllanimeOkruMerge(db); err != nil {
		t.Fatalf("AllanimeOkruMerge on fresh DB should be a no-op, got: %v", err)
	}

	var merged domain.ScraperProvider
	if err := db.Where("name = ?", "allanime-okru").First(&merged).Error; err != nil {
		t.Fatalf("allanime-okru row missing: %v", err)
	}
	if merged.Status != domain.StatusEnabled || merged.PreferenceWeight != 35 {
		t.Errorf("allanime-okru = %v/%d, want unchanged enabled/35", merged.Status, merged.PreferenceWeight)
	}
	var old domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&old)
	if old.Status != domain.StatusDisabled {
		t.Errorf("allanime status = %v, want disabled", old.Status)
	}
}

// TestFreshDB_AllanimeTombstonedDisabled reproduces the REAL fresh-DB boot
// order — AllanimeOkruMerge (no-op: empty table, nothing to rename/tombstone
// yet) followed by SeedDefaults (inserts allanime disabled + allanime-okru
// enabled/35) — and locks in the TERMINAL state so no later migration (e.g.
// the retired AllAnimeDegrade, which used to run AFTER SeedDefaults and would
// flip the freshly-seeded disabled allanime row back to degraded) can regress
// the tombstone. allanime MUST stay disabled, not degraded.
func TestFreshDB_AllanimeTombstonedDisabled(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Empty table — the real fresh-DB boot order: merge runs BEFORE seed.
	if err := scraperprovider.AllanimeOkruMerge(db); err != nil {
		t.Fatalf("AllanimeOkruMerge: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	var allanimeCount int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime").Count(&allanimeCount)
	if allanimeCount != 1 {
		t.Fatalf("allanime row count = %d, want exactly 1", allanimeCount)
	}
	var allanime domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&allanime)
	if allanime.Status != domain.StatusDisabled {
		t.Errorf("allanime status = %q, want disabled (NOT degraded — tombstone must not be revived)", allanime.Status)
	}

	var merged domain.ScraperProvider
	if err := db.Where("name = ?", "allanime-okru").First(&merged).Error; err != nil {
		t.Fatalf("allanime-okru row missing: %v", err)
	}
	if merged.Status != domain.StatusEnabled || merged.PreferenceWeight != 35 {
		t.Errorf("allanime-okru = %v/%d, want enabled/35", merged.Status, merged.PreferenceWeight)
	}

	var okruCount int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "okru").Count(&okruCount)
	if okruCount != 0 {
		t.Errorf("okru row count = %d, want 0 (no legacy row on a fresh DB)", okruCount)
	}
}

func TestAllanimeOkruCryptoBlock_RefreshesDescriptionOnceIdempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-existing live-DB state: allanime-okru manual+down with the probe-managed
	// generic reason and the old (pre-crypto-gate) description.
	if err := db.Create(&domain.ScraperProvider{
		Name: "allanime-okru", Status: domain.StatusDegraded, Policy: domain.PolicyManual, Health: domain.HealthDown,
		Reason:      "cdn_unreachable on ",
		Description: "Folded okru+allanime (2026-07-06). AllAnime GraphQL discovery + ok.ru data-options → okcdn.ru HLS, bypassing the Cloudflare-Turnstile /apivtwo/clock endpoint. EN sub/dub, hardsubbed.",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.AllanimeOkruCryptoBlock(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	var row domain.ScraperProvider
	db.Where("name = ?", "allanime-okru").First(&row)
	if !strings.Contains(row.Description, "AA_CRYPTO_MISSING") {
		t.Fatalf("description not refreshed with the crypto-gate finding: %q", row.Description)
	}
	// reason/status/policy/health are NOT this migration's concern — untouched.
	if row.Reason != "cdn_unreachable on " {
		t.Fatalf("reason should be untouched (probe-managed): %q", row.Reason)
	}
	if row.Policy != domain.PolicyManual || row.Health != domain.HealthDown {
		t.Fatalf("policy/health should be untouched (state-machine-owned): policy=%q health=%q", row.Policy, row.Health)
	}

	// A later operator's own description edit must NOT be clobbered by a rerun
	// (guard already written).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime-okru").Update("description", "operator note")
	if err := scraperprovider.AllanimeOkruCryptoBlock(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	db.Where("name = ?", "allanime-okru").First(&row)
	if row.Description != "operator note" {
		t.Fatalf("description = %q after operator edit + rerun, want untouched (not clobbered)", row.Description)
	}
}

func TestBumpKodikNoadsPriority_RaisesWeightOnceIdempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate a pre-existing live DB where kodik-noads still sits at the old
	// dead-last weight (fresh DBs already seed 90; the migration must also carry
	// prod rows created before the seed bump).
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "kodik-noads").
		Update("preference_weight", 0).Error; err != nil {
		t.Fatalf("preset kodik-noads weight=0: %v", err)
	}

	if err := scraperprovider.BumpKodikNoadsPriority(db); err != nil {
		t.Fatalf("bump kodik-noads priority: %v", err)
	}

	var noads, ae, sibnet, allvideo domain.ScraperProvider
	db.First(&noads, "name = ?", "kodik-noads")
	db.First(&ae, "name = ?", "ae")
	db.First(&sibnet, "name = ?", "animejoy-sibnet")
	db.First(&allvideo, "name = ?", "animejoy-allvideo")
	if noads.PreferenceWeight != 90 {
		t.Errorf("kodik-noads preference_weight = %d, want 90", noads.PreferenceWeight)
	}
	// The whole point: under ae, above the AnimeJoy RU-sub legs.
	if !(noads.PreferenceWeight < ae.PreferenceWeight) {
		t.Errorf("kodik-noads (%d) must rank UNDER ae (%d)", noads.PreferenceWeight, ae.PreferenceWeight)
	}
	if !(noads.PreferenceWeight > sibnet.PreferenceWeight && noads.PreferenceWeight > allvideo.PreferenceWeight) {
		t.Errorf("kodik-noads (%d) must rank ABOVE sibnet (%d) and allvideo (%d)",
			noads.PreferenceWeight, sibnet.PreferenceWeight, allvideo.PreferenceWeight)
	}

	// A later operator re-tune must NOT be clobbered by a rerun (guard written).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "kodik-noads").Update("preference_weight", 42)
	if err := scraperprovider.BumpKodikNoadsPriority(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	db.First(&noads, "name = ?", "kodik-noads")
	if noads.PreferenceWeight != 42 {
		t.Errorf("kodik-noads weight = %d after operator re-tune + rerun, want 42 (not clobbered)", noads.PreferenceWeight)
	}
}

func TestBumpKodikNoadsPriority_NoRow_ErrorsAndDoesNotWriteGuard(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// No kodik-noads row exists — the migration must error and NOT write its guard
	// so a later boot (after the row is seeded) retries.
	if err := scraperprovider.BumpKodikNoadsPriority(db); err == nil {
		t.Fatal("expected an error when kodik-noads row is absent")
	}
	var guards int64
	db.Table("catalog_migration_guards").
		Where("key = ?", "bump_kodik_noads_priority_90_2026_07_07").Count(&guards)
	if guards != 0 {
		t.Errorf("guard written despite no row found (count=%d); a later boot must retry", guards)
	}
}

func TestReconcilePolicyFromHealthV1_AlignsRows(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed a mix: auto+down (should park), manual+up (should un-park), an
	// up/degraded/recovering set, and a disabled row (must be immune).
	rows := []domain.ScraperProvider{
		{Name: "animepahe", Policy: domain.PolicyAuto, Health: domain.HealthDown, Status: domain.StatusDegraded},
		{Name: "miruro", Policy: domain.PolicyManual, Health: domain.HealthUp, Status: domain.StatusDegraded},
		{Name: "nineanime", Policy: domain.PolicyAuto, Health: domain.HealthRecovering, Status: domain.StatusDegraded},
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthDegraded, Status: domain.StatusEnabled},
		{Name: "animefever", Policy: domain.PolicyDisabled, Health: domain.HealthDown, Status: domain.StatusDisabled},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := scraperprovider.ReconcilePolicyFromHealthV1(db); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	want := map[string]struct {
		policy domain.ProviderPolicy
		status domain.ProviderStatus
	}{
		"animepahe":  {domain.PolicyManual, domain.StatusDegraded}, // down -> manual
		"miruro":     {domain.PolicyAuto, domain.StatusEnabled},    // up -> auto (rejoins chain)
		"nineanime":  {domain.PolicyAuto, domain.StatusDegraded},   // recovering -> auto, soaks (WireStatus degraded)
		"gogoanime":  {domain.PolicyAuto, domain.StatusEnabled},    // degraded -> auto, one-blip buffer keeps it enabled
		"animefever": {domain.PolicyDisabled, domain.StatusDisabled}, // disabled untouched
	}
	for name, w := range want {
		var got domain.ScraperProvider
		if err := db.First(&got, "name = ?", name).Error; err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if got.Policy != w.policy || got.Status != w.status {
			t.Errorf("%s: (policy,status)=(%s,%s) want (%s,%s)", name, got.Policy, got.Status, w.policy, w.status)
		}
	}

	// Idempotent: a second run is a guarded no-op even if a later machine write
	// changed a row (we mutate animepahe to auto and confirm it is NOT clobbered).
	if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", "animepahe").Update("policy", domain.PolicyAuto).Error; err != nil {
		t.Fatal(err)
	}
	if err := scraperprovider.ReconcilePolicyFromHealthV1(db); err != nil {
		t.Fatalf("reconcile 2nd: %v", err)
	}
	var again domain.ScraperProvider
	db.First(&again, "name = ?", "animepahe")
	if again.Policy != domain.PolicyAuto {
		t.Errorf("guarded re-run clobbered a later write: animepahe policy=%s want auto", again.Policy)
	}
}

func TestAllanimeOkruCryptoGateLifted_RefreshesDescriptionOnce(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name:        "allanime-okru",
		Policy:      domain.PolicyManual,
		Health:      domain.HealthDown,
		Status:      domain.StatusDegraded,
		Reason:      "cdn_unreachable on ",
		Description: "stale AA_CRYPTO_MISSING tombstone",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraperprovider.AllanimeOkruCryptoGateLifted(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var got domain.ScraperProvider
	db.First(&got, "name = ?", "allanime-okru")
	if !strings.Contains(got.Description, "LIFTED") {
		t.Errorf("description not refreshed: %q", got.Description)
	}
	// reason/status/policy/health untouched (machine/admin-owned).
	if got.Reason != "cdn_unreachable on " || got.Policy != domain.PolicyManual || got.Health != domain.HealthDown {
		t.Errorf("touched a machine/admin field: %+v", got)
	}

	// Idempotent: a later operator edit is not clobbered on a second run.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime-okru").Update("description", "operator note")
	if err := scraperprovider.AllanimeOkruCryptoGateLifted(db); err != nil {
		t.Fatalf("2nd run: %v", err)
	}
	db.First(&got, "name = ?", "allanime-okru")
	if got.Description != "operator note" {
		t.Errorf("guarded re-run clobbered operator edit: %q", got.Description)
	}
}

func TestAllanimeOkruCryptoGateLifted_NoRowNoGuard(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// No allanime-okru row → migration must error and NOT write its guard, so a
	// later boot (after the row exists) retries.
	if err := scraperprovider.AllanimeOkruCryptoGateLifted(db); err == nil {
		t.Fatal("expected error when row absent")
	}
}
