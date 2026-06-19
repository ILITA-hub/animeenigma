package scraperprovider

import (
	"fmt"
	"time"

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

// migrationGuard is a tiny key→applied ledger that records one-time data
// migrations so they don't re-run on every boot. Kept in its OWN table (not a
// sentinel row inside stream_providers) so it can never leak into a roster read /
// served provider list / Prometheus series. Mirrors the schema-state guard
// philosophy of RenameScraperProvidersTable, but for a status-flip that has no
// schema signal of its own.
type migrationGuard struct {
	Key       string    `gorm:"primaryKey;size:64"`
	AppliedAt time.Time `gorm:"autoCreateTime"`
}

func (migrationGuard) TableName() string { return "catalog_migration_guards" }

// retireHanimeAnimelibGuardKey marks RetireHanimeAnimelib as applied.
const retireHanimeAnimelibGuardKey = "retire_hanime_animelib"

// RetireHanimeAnimelib disables the hanime + animelib roster rows exactly once
// (Plan B: those player surfaces are retired and their content dropped). RUN-ONCE
// guarded via the catalog_migration_guards ledger, so on every subsequent boot it
// is a no-op and an operator who later re-enables either row in the DB is NOT
// clobbered. All other rows (ae, kodik, raw, 18anime, the EN scraper chain) are
// untouched. Idempotent; safe to call every boot.
func RetireHanimeAnimelib(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", retireHanimeAnimelibGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check retire-hanime-animelib guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber later operator re-enables
	}

	if err := db.Model(&domain.ScraperProvider{}).
		Where("name IN ?", []string{"hanime", "animelib"}).
		Update("status", domain.StatusDisabled).Error; err != nil {
		return fmt.Errorf("retire hanime+animelib (status=disabled): %w", err)
	}

	if err := db.Create(&migrationGuard{Key: retireHanimeAnimelibGuardKey}).Error; err != nil {
		return fmt.Errorf("write retire-hanime-animelib guard: %w", err)
	}
	return nil
}

// reEnableHanimeGuardKey marks ReEnableHanime as applied.
const reEnableHanimeGuardKey = "reenable_hanime"

// ReEnableHanime re-enables the hanime roster row exactly once. Forward-only
// counterpart to RetireHanimeAnimelib: hanime was retired in Plan B (2026-06-18)
// but restored as an in-aePlayer 18+ source (2026-06-19). RUN-ONCE guarded via
// the catalog_migration_guards ledger, so on every subsequent boot it is a no-op
// and an operator who later re-disables hanime is NOT clobbered. Must run AFTER
// SeedDefaults + RetireHanimeAnimelib so it wins the final status on fresh DBs
// (seed=enabled -> retire disables -> this re-enables). animelib is intentionally
// left disabled. Idempotent; safe to call every boot.
func ReEnableHanime(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", reEnableHanimeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check reenable-hanime guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-disable
	}

	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "hanime").
		Update("status", domain.StatusEnabled)
	if result.Error != nil {
		return fmt.Errorf("re-enable hanime (status=enabled): %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No hanime row to flip (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("re-enable hanime: no row found for name=hanime")
	}

	if err := db.Create(&migrationGuard{Key: reEnableHanimeGuardKey}).Error; err != nil {
		return fmt.Errorf("write reenable-hanime guard: %w", err)
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

