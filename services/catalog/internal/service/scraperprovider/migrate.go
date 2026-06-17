package scraperprovider

import (
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// RenameScraperProvidersTable renames the legacy scraper_providers table to
// stream_providers exactly once. Guarded: only renames when the old table exists
// and the new one does not, so it is a no-op on fresh DBs and on every boot after
// the rename. Must run BEFORE AutoMigrate(&domain.ScraperProvider{}) so the new
// scraper_operated column is added to the renamed (data-carrying) table rather
// than to a fresh empty stream_providers. Works on SQLite (tests) + Postgres.
func RenameScraperProvidersTable(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable("scraper_providers") && !m.HasTable("stream_providers") {
		if err := db.Exec("ALTER TABLE scraper_providers RENAME TO stream_providers").Error; err != nil {
			return fmt.Errorf("rename scraper_providers -> stream_providers: %w", err)
		}
	}
	return nil
}

// BackfillScraperOperated sets the intrinsic scraper_operated flag on every row.
// Idempotent and safe to run every boot: like Group, the flag is intrinsic (NOT
// operator-editable), so re-deriving it from the canonical name set is always
// correct. Bounded row count (~13).
func BackfillScraperOperated(db *gorm.DB) error {
	names := scraperOperatedNameList()
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name IN ?", names).Update("scraper_operated", true).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=true: %w", err)
	}
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name NOT IN ?", names).Update("scraper_operated", false).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=false: %w", err)
	}
	return nil
}

// scraperOperatedNameList is the canonical set of scraper-microservice-operated
// provider names. TEMPORARY: this stub lives here so Task 1 compiles and its tests
// pass. Task 2 moves the authoritative version to seed.go; at that point this
// function is deleted from migrate.go.
func scraperOperatedNameList() []string {
	return []string{
		"gogoanime", "animepahe", "allanime", "animefever",
		"miruro", "nineanime", "animekai", "18anime",
	}
}
