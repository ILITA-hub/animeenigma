package gormtrace

import (
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// effectWidget is the test model — a trivial table to exercise CRUD effects.
type effectWidget struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (effectWidget) TableName() string { return "effect_widgets" }

// fakeSink is a capturing, concurrency-safe tracing.EffectSink for assertions.
type fakeSink struct {
	mu      sync.Mutex
	effects []tracing.Effect
}

func (s *fakeSink) Record(e tracing.Effect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.effects = append(s.effects, e)
}

func (s *fakeSink) all() []tracing.Effect {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tracing.Effect, len(s.effects))
	copy(out, s.effects)
	return out
}

func (s *fakeSink) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.effects = nil
}

func (s *fakeSink) kind(k string) []tracing.Effect {
	var out []tracing.Effect
	for _, e := range s.all() {
		if e.EffectKind == k {
			out = append(out, e)
		}
	}
	return out
}

// alwaysGate forces the read-gate decision for read-path tests.
type alwaysGate bool

func (g alwaysGate) ShouldRecord(operation, table string, durationMS int) bool {
	return bool(g)
}

// newEffectDB builds an in-memory sqlite GORM DB with the effect callbacks
// registered against sink + gate.
func newEffectDB(t *testing.T, sink tracing.EffectSink, gate ReadGate) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&effectWidget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := RegisterEffectCallbacks(db, sink, gate); err != nil {
		t.Fatalf("RegisterEffectCallbacks: %v", err)
	}
	return db
}

func TestDBWriteEffect(t *testing.T) {
	t.Run("Create emits one db_write with Rows>=1", func(t *testing.T) {
		sink := &fakeSink{}
		// gate=false so AutoMigrate's reads (if any after registration) never leak;
		// writes are never gated.
		db := newEffectDB(t, sink, alwaysGate(false))
		sink.reset()

		if err := db.Create(&effectWidget{Name: "a"}).Error; err != nil {
			t.Fatalf("create: %v", err)
		}

		writes := sink.kind("db_write")
		if len(writes) != 1 {
			t.Fatalf("want 1 db_write, got %d (all=%+v)", len(writes), sink.all())
		}
		e := writes[0]
		if e.TargetKind != "table" {
			t.Errorf("TargetKind = %q, want table", e.TargetKind)
		}
		if e.Target != "effect_widgets" {
			t.Errorf("Target = %q, want effect_widgets", e.Target)
		}
		if e.Rows < 1 {
			t.Errorf("Rows = %d, want >=1", e.Rows)
		}
		if e.Requests != 1 {
			t.Errorf("Requests = %d, want 1", e.Requests)
		}
		if e.EffectKind == "" {
			t.Errorf("EffectKind must never be empty")
		}
	})

	t.Run("Update affecting N rows emits db_write Rows=N (RowsAffected not zeroed)", func(t *testing.T) {
		sink := &fakeSink{}
		db := newEffectDB(t, sink, alwaysGate(false))
		// Seed three rows.
		db.Create(&effectWidget{Name: "x"})
		db.Create(&effectWidget{Name: "x"})
		db.Create(&effectWidget{Name: "x"})
		sink.reset()

		if err := db.Model(&effectWidget{}).Where("name = ?", "x").Update("name", "y").Error; err != nil {
			t.Fatalf("update: %v", err)
		}

		writes := sink.kind("db_write")
		if len(writes) != 1 {
			t.Fatalf("want 1 db_write, got %d", len(writes))
		}
		if writes[0].Rows != 3 {
			t.Errorf("Rows = %d, want 3 (RowsAffected must not be zeroed by an in-callback query — Pitfall 1)", writes[0].Rows)
		}
	})

	t.Run("Delete emits db_write", func(t *testing.T) {
		sink := &fakeSink{}
		db := newEffectDB(t, sink, alwaysGate(false))
		w := effectWidget{Name: "d"}
		db.Create(&w)
		sink.reset()

		if err := db.Delete(&effectWidget{}, w.ID).Error; err != nil {
			t.Fatalf("delete: %v", err)
		}
		writes := sink.kind("db_write")
		if len(writes) != 1 {
			t.Fatalf("want 1 db_write on delete, got %d", len(writes))
		}
		if writes[0].EffectKind != "db_write" {
			t.Errorf("EffectKind = %q, want db_write", writes[0].EffectKind)
		}
	})
}

func TestDBReadEffect(t *testing.T) {
	t.Run("gated-false SELECT emits no effect", func(t *testing.T) {
		sink := &fakeSink{}
		db := newEffectDB(t, sink, alwaysGate(false))
		db.Create(&effectWidget{Name: "r"})
		sink.reset()

		var got []effectWidget
		if err := db.Find(&got).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if reads := sink.kind("db_read"); len(reads) != 0 {
			t.Fatalf("want 0 db_read when gate says no, got %d", len(reads))
		}
	})

	t.Run("gated-true SELECT emits one db_read with non-zero Rows", func(t *testing.T) {
		sink := &fakeSink{}
		db := newEffectDB(t, sink, alwaysGate(true))
		db.Create(&effectWidget{Name: "r"})
		db.Create(&effectWidget{Name: "r"})
		sink.reset()

		var got []effectWidget
		if err := db.Find(&got).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		reads := sink.kind("db_read")
		if len(reads) != 1 {
			t.Fatalf("want 1 db_read when gate says yes, got %d (all=%+v)", len(reads), sink.all())
		}
		e := reads[0]
		if e.EffectKind != "db_read" {
			t.Errorf("EffectKind = %q, want db_read", e.EffectKind)
		}
		if e.TargetKind != "table" {
			t.Errorf("TargetKind = %q, want table", e.TargetKind)
		}
		if e.Target != "effect_widgets" {
			t.Errorf("Target = %q, want effect_widgets", e.Target)
		}
		if e.Rows < 1 {
			t.Errorf("Rows = %d, want >=1 (A6: reflect-len of Dest when RowsAffected==0)", e.Rows)
		}
	})

	t.Run("EffectKind always explicit, never empty", func(t *testing.T) {
		sink := &fakeSink{}
		db := newEffectDB(t, sink, alwaysGate(true))
		db.Create(&effectWidget{Name: "k"})
		var got []effectWidget
		db.Find(&got)
		for _, e := range sink.all() {
			if e.EffectKind == "" {
				t.Fatalf("found an effect with empty EffectKind: %+v", e)
			}
		}
	})
}
