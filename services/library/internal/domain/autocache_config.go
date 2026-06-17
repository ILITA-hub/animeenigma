package domain

import "time"

// AutocacheConfig is the Go-side mirror of the singleton autocache_config
// row defined in migrations/006_autocache_config.sql. Exactly ONE row
// exists (id fixed at 1 by a PK default + CHECK constraint), holding the
// live-editable §3.5 tunables and the master `enabled` switch.
//
// POOL-04 lets an admin GET/PATCH these values with no redeploy; POOL-05
// stores the master `enabled` switch that the future downloader/evictor
// (Phases 8-10) will consume via the typed Get/Patch accessor. This phase
// only persists + serves the config — it implements no download/eviction
// behavior.
//
// Field tags use snake_case `column:` to match the SQL columns 1:1, with
// `not null` mirroring the NOT NULL columns. GORM AutoMigrate is NOT used
// (the SQL migration is the source of truth); the GORM defaults are kept
// in sync only so AutoMigrate would stay a safe no-op.
type AutocacheConfig struct {
	ID                    int       `gorm:"primaryKey;column:id;default:1" json:"-"`
	Enabled               bool      `gorm:"not null;column:enabled;default:true" json:"enabled"`
	BudgetBytes           int64     `gorm:"not null;column:budget_bytes;default:107374182400" json:"budget_bytes"`
	AutoFreshDownloadDays int       `gorm:"not null;column:auto_fresh_download_days;default:10" json:"auto_fresh_download_days"`
	AutoFreshFetchDays    int       `gorm:"not null;column:auto_fresh_fetch_days;default:3" json:"auto_fresh_fetch_days"`
	AdminFreshDays        int       `gorm:"not null;column:admin_fresh_days;default:30" json:"admin_fresh_days"`
	ActiveWatcherDays     int       `gorm:"not null;column:active_watcher_days;default:30" json:"active_watcher_days"`
	QualityCap            int       `gorm:"not null;column:quality_cap;default:1080" json:"quality_cap"`
	MinSeeders            int       `gorm:"not null;column:min_seeders;default:3" json:"min_seeders"`
	SweepIntervalMin      int       `gorm:"not null;column:sweep_interval_min;default:20" json:"sweep_interval_min"`
	UpdatedAt             time.Time `gorm:"not null;column:updated_at" json:"updated_at"`
}

// TableName pins the table name (GORM would otherwise pluralize). The
// migration uses "autocache_config".
func (AutocacheConfig) TableName() string { return "autocache_config" }
