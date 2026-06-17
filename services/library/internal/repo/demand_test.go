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
// the ON CONFLICT (mal_id, episode) DO UPDATE upsert (the dedup contract). The
// behavioral DB assertion lives behind //go:build integration; this is the no-DB
// source tripwire so a refactor can't quietly drop the dedup clause.
func TestDemandRepository_Record_UpsertTripwire(t *testing.T) {
	src, err := os.ReadFile("demand.go")
	if err != nil {
		t.Fatalf("read demand.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "clause.OnConflict") {
		t.Fatal("demand.go Record must use clause.OnConflict (dedup upsert tripwire)")
	}
	if !strings.Contains(s, `"requested_at"`) {
		t.Fatal("demand.go Record must refresh requested_at on conflict (recency tripwire)")
	}
}
