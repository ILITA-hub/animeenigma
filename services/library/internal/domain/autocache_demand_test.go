package domain

import (
	"reflect"
	"testing"
)

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
	// ongoing (Logic A — scheduler ongoing-push) added in Phase 09 / migration 010;
	// distinct from next_ep (Logic B) and backfill so OBS-04 attributes downloads.
	if DemandReasonOngoing != "ongoing" {
		t.Fatalf("DemandReasonOngoing = %q, want %q", DemandReasonOngoing, "ongoing")
	}
}

// TestAutocacheDemandTableName pins the table name so a future refactor that
// accidentally pluralizes to "autocache_demands" is caught.
func TestAutocacheDemandTableName(t *testing.T) {
	if got := (AutocacheDemand{}).TableName(); got != "autocache_demand" {
		t.Fatalf("AutocacheDemand.TableName() = %q, want %q", got, "autocache_demand")
	}
}

// TestJobEpisodeFieldIsNullableInt pins the Phase-09 single-flight key: Job.Episode
// must be a *int (nullable pointer — absent for admin/manual rows, set by the
// Planner) carrying the column:episode GORM tag. A refactor that drops the pointer
// (making admin rows write episode=0 instead of NULL) is caught here.
func TestJobEpisodeFieldIsNullableInt(t *testing.T) {
	f, ok := reflect.TypeOf(Job{}).FieldByName("Episode")
	if !ok {
		t.Fatal("Job.Episode field missing")
	}
	if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Kind() != reflect.Int {
		t.Fatalf("Job.Episode must be *int (nullable), got %s", f.Type)
	}
	if got := f.Tag.Get("gorm"); !reflect.DeepEqual(got != "" && containsSub(got, "column:episode"), true) {
		t.Fatalf("Job.Episode gorm tag must contain column:episode, got %q", got)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
