package domain

import (
	"strings"
	"time"
)

// DemandReason is the Go-side mirror of the autocache_demand_reason pg enum
// (migrations/007_autocache_demand.sql). The named-string convention matches
// EpisodeSource / EpisodeTrack from migration 005 — the value is stored as the
// enum literal, so the strings MUST match the SQL labels exactly.
//
// Phase 08 only ever wrote 'backfill' (the serve MISS path). Phase 09 adds the
// two watch-driven producers: Logic B (player next-episode pull) writes
// 'next_ep' and Logic A (scheduler ongoing-push) writes 'ongoing' — three
// distinct reasons so OBS-04 can attribute downloads by trigger.
type DemandReason string

const (
	// DemandReasonNextEp is Logic B (Phase 09): the player fired a demand for
	// episode N+1 ahead of an active JP-audio watcher.
	DemandReasonNextEp DemandReason = "next_ep"
	// DemandReasonBackfill is the ae serve MISS path (Phase 08): the episode was
	// absent from the pool, so it is wanted for backfill.
	DemandReasonBackfill DemandReason = "backfill"
	// DemandReasonOngoing is Logic A (Phase 09): the scheduler ongoing-push
	// re-asserts demand for the latest-aired episode of an ongoing anime that has
	// ≥1 active JP-audio watcher. Stored as the 'ongoing' enum literal added by
	// migrations/010_autocache_demand_ongoing.sql.
	DemandReasonOngoing DemandReason = "ongoing"
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
	// Titles is the newline-delimited, ordered list of anime titles the producer
	// supplied (name_jp → romaji → name_en, non-empty + deduped). The library has
	// no anime table of its own, so this is the ONLY way the Planner can build a
	// real tracker search query (migration 011). Empty for legacy/title-less rows.
	Titles string `gorm:"not null;column:titles;default:''" json:"titles"`
}

// TableName pins the table name (GORM would otherwise pluralize). The migration
// uses "autocache_demand".
func (AutocacheDemand) TableName() string { return "autocache_demand" }

// titleSep is the in-column delimiter for AutocacheDemand.Titles. Anime titles
// never contain a newline, so this round-trips losslessly on Postgres + SQLite.
const titleSep = "\n"

// SearchTitles returns the ordered title list (name_jp → romaji → name_en) the
// Planner tries in turn. Empty slice for a legacy/title-less row (the Planner
// then falls back to the mal_id query).
func (d AutocacheDemand) SearchTitles() []string {
	if d.Titles == "" {
		return nil
	}
	return strings.Split(d.Titles, titleSep)
}

// JoinTitles normalizes an ordered title list into the column form: trims each,
// drops empties, dedups (preserving order), and joins with titleSep. Producers
// pass [name_jp, name, name_en]; this guards against blank/duplicate fields.
func JoinTitles(titles []string) string {
	seen := make(map[string]struct{}, len(titles))
	out := make([]string, 0, len(titles))
	for _, t := range titles {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return strings.Join(out, titleSep)
}
