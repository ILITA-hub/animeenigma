package domain

import "time"

// DemandReason is the Go-side mirror of the autocache_demand_reason pg enum
// (migrations/007_autocache_demand.sql). The named-string convention matches
// EpisodeSource / EpisodeTrack from migration 005 — the value is stored as the
// enum literal, so the strings MUST match the SQL labels exactly.
//
// 'next_ep' is RESERVED for Phase 09's next-episode predictor: it is present in
// the enum so no schema change is needed later, but Phase 08 only ever writes
// 'backfill' (the serve MISS path).
type DemandReason string

const (
	// DemandReasonNextEp is reserved for Phase 09 (next-episode prediction) —
	// declared now, never written in this milestone phase.
	DemandReasonNextEp DemandReason = "next_ep"
	// DemandReasonBackfill is the only reason written in Phase 08: the ae serve
	// path missed the pool, so the episode is wanted for backfill.
	DemandReasonBackfill DemandReason = "backfill"
)

// AutocacheDemand is the Go-side mirror of an autocache_demand row defined in
// migrations/007_autocache_demand.sql. One row exists per wanted
// (mal_id, episode) — the composite primary key dedups concurrent demand, and
// DemandRepository.Record refreshes RequestedAt via ON CONFLICT DO UPDATE.
//
// Field tags use snake_case `column:` to match the SQL columns 1:1, the two
// key columns carry `primaryKey`, and Reason uses the `type:<pg_enum>` tag
// convention so GORM addresses the autocache_demand_reason enum. GORM
// AutoMigrate is NOT used (the SQL migration is the source of truth);
// mal_id == shikimori_id (CONTEXT line 42).
type AutocacheDemand struct {
	MALID       string       `gorm:"primaryKey;column:mal_id" json:"mal_id"`
	Episode     int          `gorm:"primaryKey;column:episode" json:"episode"`
	Reason      DemandReason `gorm:"not null;column:reason;type:autocache_demand_reason" json:"reason"`
	RequestedAt time.Time    `gorm:"not null;column:requested_at" json:"requested_at"`
}

// TableName pins the table name (GORM would otherwise pluralize). The migration
// uses "autocache_demand".
func (AutocacheDemand) TableName() string { return "autocache_demand" }
