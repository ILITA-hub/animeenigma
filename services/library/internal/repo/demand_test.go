package repo

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// TestNewDemandRepository_NonNil — the constructor returns a usable repo over
// the provided *gorm.DB. The DB-backed upsert/dedup behavior is integration-
// gated (the repo package's behavioral tests run behind //go:build integration
// per 07-02/07-03); this no-DB test guards the constructor wiring.
func TestNewDemandRepository_NonNil(t *testing.T) {
	if got := NewDemandRepository(&gorm.DB{}); got == nil {
		t.Fatal("NewDemandRepository returned nil")
	}
}

// TestDemandRepository_Record_Signature pins the Record method shape so a
// refactor can't silently reshape the upsert entry point the serve MISS path
// depends on: (recv, ctx, malID string, episode int, reason DemandReason) → error.
func TestDemandRepository_Record_Signature(t *testing.T) {
	rt := reflect.TypeOf(&DemandRepository{})
	m, ok := rt.MethodByName("Record")
	if !ok {
		t.Fatal("DemandRepository.Record missing")
	}
	// (recv, ctx, malID, episode, reason) → error
	if got := m.Type.NumIn(); got != 5 {
		t.Fatalf("Record NumIn = %d, want 5 (recv, ctx, malID, episode, reason)", got)
	}
	if m.Type.In(2).Kind() != reflect.String {
		t.Fatalf("Record malID arg must be string, got %s", m.Type.In(2))
	}
	if m.Type.In(3).Kind() != reflect.Int {
		t.Fatalf("Record episode arg must be int, got %s", m.Type.In(3))
	}
	if m.Type.In(4) != reflect.TypeOf(domain.DemandReason("")) {
		t.Fatalf("Record reason arg must be domain.DemandReason, got %s", m.Type.In(4))
	}
	if m.Type.NumOut() != 1 || !m.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatal("Record must return a single error")
	}
}

// TestDemandRepository_Record_UpsertTripwire guards that Record actually performs
// the ON CONFLICT (mal_id, episode) DO UPDATE upsert (the dedup contract) and, on
// conflict, refreshes `reason` (WR-02). It also guards that the conflict path does
// NOT re-stamp requested_at (WR-01) — the original first-seen time is the stable
// FIFO key. The behavioral DB assertions live in the sqlite tests below; this is
// the no-DB source tripwire so a refactor can't quietly drop either invariant.
func TestDemandRepository_Record_UpsertTripwire(t *testing.T) {
	src, err := os.ReadFile("demand.go")
	if err != nil {
		t.Fatalf("read demand.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "clause.OnConflict") {
		t.Fatal("demand.go Record must use clause.OnConflict (dedup upsert tripwire)")
	}
	if !strings.Contains(s, `AssignmentColumns([]string{"reason"})`) {
		t.Fatal("demand.go Record must refresh reason on conflict (WR-02 trigger-attribution tripwire)")
	}
	// WR-01: requested_at must NOT be reassigned in the DoUpdates set — a re-assert
	// keeps the original first-seen time so the FIFO drain ordering stays stable.
	if strings.Contains(s, `"requested_at": gorm.Expr`) {
		t.Fatal("demand.go Record must NOT bump requested_at on conflict (WR-01 FIFO-starvation regression)")
	}
}

// TestDemandRepository_Drain_Signature pins the Phase-09 drain primitive shape:
// (recv, ctx, limit int) → ([]domain.AutocacheDemand, error). DB-backed ordering
// behavior is integration-gated; this guards the signature the Planner imports.
func TestDemandRepository_Drain_Signature(t *testing.T) {
	rt := reflect.TypeOf(&DemandRepository{})
	m, ok := rt.MethodByName("Drain")
	if !ok {
		t.Fatal("DemandRepository.Drain missing")
	}
	if got := m.Type.NumIn(); got != 3 {
		t.Fatalf("Drain NumIn = %d, want 3 (recv, ctx, limit)", got)
	}
	if m.Type.In(2).Kind() != reflect.Int {
		t.Fatalf("Drain limit arg must be int, got %s", m.Type.In(2))
	}
	if m.Type.NumOut() != 2 {
		t.Fatalf("Drain NumOut = %d, want 2 ([]AutocacheDemand, error)", m.Type.NumOut())
	}
	if m.Type.Out(0) != reflect.TypeOf([]domain.AutocacheDemand(nil)) {
		t.Fatalf("Drain first return must be []domain.AutocacheDemand, got %s", m.Type.Out(0))
	}
	if !m.Type.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatal("Drain second return must be error")
	}
}

// TestDemandRepository_Delete_Signature pins the drain-companion delete primitive:
// (recv, ctx, malID string, episode int) → error. Deleting an absent row is a
// no-op (integration-tested); this guards the signature.
func TestDemandRepository_Delete_Signature(t *testing.T) {
	rt := reflect.TypeOf(&DemandRepository{})
	m, ok := rt.MethodByName("Delete")
	if !ok {
		t.Fatal("DemandRepository.Delete missing")
	}
	if got := m.Type.NumIn(); got != 4 {
		t.Fatalf("Delete NumIn = %d, want 4 (recv, ctx, malID, episode)", got)
	}
	if m.Type.In(2).Kind() != reflect.String {
		t.Fatalf("Delete malID arg must be string, got %s", m.Type.In(2))
	}
	if m.Type.In(3).Kind() != reflect.Int {
		t.Fatalf("Delete episode arg must be int, got %s", m.Type.In(3))
	}
	if m.Type.NumOut() != 1 || !m.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatal("Delete must return a single error")
	}
}

// TestDemandRepository_Drain_OrderTripwire guards that Drain orders oldest-first by
// requested_at (the Planner FIFO contract) and bounds the result with a Limit. The
// behavioral DB assertion is integration-gated; this is the no-DB source tripwire.
func TestDemandRepository_Drain_OrderTripwire(t *testing.T) {
	src, err := os.ReadFile("demand.go")
	if err != nil {
		t.Fatalf("read demand.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, `Order("requested_at ASC")`) {
		t.Fatal("demand.go Drain must Order by requested_at ASC (FIFO drain tripwire)")
	}
	if !strings.Contains(s, "Limit(limit)") {
		t.Fatal("demand.go Drain must bound the batch with Limit(limit) (DoS guard T-09-02)")
	}
}
