package domain

import "testing"

// The DemandReason const string values MUST match the autocache_demand_reason
// enum labels in migrations/007_autocache_demand.sql exactly (the GORM model
// mirrors the SQL 1:1 — no AutoMigrate). 'next_ep' is reserved for Phase 09 but
// is declared now, so both values are pinned.
func TestDemandReasonValues(t *testing.T) {
	if DemandReasonBackfill != "backfill" {
		t.Fatalf("DemandReasonBackfill = %q, want %q", DemandReasonBackfill, "backfill")
	}
	// next_ep is reserved (Phase 09) — present in the enum, never written in P8.
	if DemandReasonNextEp != "next_ep" {
		t.Fatalf("DemandReasonNextEp = %q, want %q", DemandReasonNextEp, "next_ep")
	}
}

// TestAutocacheDemandTableName pins the table name so a future refactor that
// accidentally pluralizes to "autocache_demands" is caught.
func TestAutocacheDemandTableName(t *testing.T) {
	if got := (AutocacheDemand{}).TableName(); got != "autocache_demand" {
		t.Fatalf("AutocacheDemand.TableName() = %q, want %q", got, "autocache_demand")
	}
}
