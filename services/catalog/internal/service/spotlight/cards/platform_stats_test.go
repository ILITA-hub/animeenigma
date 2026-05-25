package cards

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// newTestDB spins an in-memory SQLite and creates a minimal `animes`
// table compatible with the resolver's Count query. domain.Anime's
// production schema uses Postgres-specific `gen_random_uuid()` defaults
// that SQLite's AutoMigrate cannot parse, so we hand-roll a CREATE TABLE
// statement with just the columns the resolver touches (id, created_at,
// deleted_at — the soft-delete filter is applied by GORM via the field's
// `gorm:"index"` tag on domain.Anime.DeletedAt).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Minimal schema — the resolver only needs created_at + a row count.
	// Other domain.Anime columns are irrelevant to the Count() under test.
	ddl := `CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		score REAL,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("create animes table: %v", err)
	}
	return db
}

func TestPlatformStats_Type(t *testing.T) {
	r := &PlatformStatsResolver{}
	if got := r.Type(); got != "platform_stats" {
		t.Errorf("Type() = %q, want %q", got, "platform_stats")
	}
}

// insertAnime adds a row to the test-DB's `animes` table via raw SQL so
// we avoid GORM trying to write Anime's full ~30-column schema (the
// minimal SQLite table only has id/name/score/created_at/updated_at/
// deleted_at).
func insertAnime(t *testing.T, db *gorm.DB, id string, createdAt time.Time) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO animes (id, name, score, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, id, 8.0, createdAt, createdAt,
	).Error
	if err != nil {
		t.Fatalf("insert %q: %v", id, err)
	}
}

func TestPlatformStats_Resolve_AllMetricsComputable_ReturnsCard(t *testing.T) {
	db := newTestDB(t)
	// Insert 3 rows inside the 7-day window, 2 outside.
	now := time.Now()
	for i := 0; i < 3; i++ {
		insertAnime(t, db, "in-"+string(rune('a'+i)), now.Add(-3*24*time.Hour))
	}
	for i := 0; i < 2; i++ {
		insertAnime(t, db, "out-"+string(rune('a'+i)), now.Add(-10*24*time.Hour))
	}

	c := newFakeCache()
	r := NewPlatformStatsResolver(db, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card with at least 1 computable metric")
	}
	if card.Type != "platform_stats" {
		t.Errorf("Card.Type = %q, want platform_stats", card.Type)
	}
	data, ok := card.Data.(spotlight.PlatformStatsData)
	if !ok {
		t.Fatalf("Card.Data not PlatformStatsData: %T", card.Data)
	}
	if len(data.Metrics) != 1 {
		t.Fatalf("expected exactly 1 metric (Phase 1 ships only anime_added_7d), got %d: %+v", len(data.Metrics), data.Metrics)
	}
	m := data.Metrics[0]
	if m.Key != "anime_added_7d" {
		t.Errorf("metric Key = %q, want anime_added_7d", m.Key)
	}
	if m.Value != 3 {
		t.Errorf("metric Value = %d, want 3 (in-window count)", m.Value)
	}
	// No entries for the Phase 1 nil metrics.
	for _, m := range data.Metrics {
		if m.Key == "episodes_added_7d" {
			t.Error("episodes_added_7d should be SKIPPED in Phase 1 (no event log)")
		}
		if m.Key == "active_rooms_7d" {
			t.Error("active_rooms_7d should be SKIPPED in Phase 1 (rooms is Redis-only)")
		}
	}
	// Cache write happened.
	if c.sets != 1 {
		t.Errorf("expected 1 cache.Set, got %d", c.sets)
	}
}

// TestPlatformStats_Resolve_EmitsSeriesAndPreviousValue asserts the
// v1.1-polish Phase 08 enrichment (HSB-V11-PS-01): every emitted metric
// carries a 7-element Series and a non-nil PreviousValue.
func TestPlatformStats_Resolve_EmitsSeriesAndPreviousValue(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	// Rows inside the trailing 7-day window (counted in Value + Series).
	for i := 0; i < 3; i++ {
		insertAnime(t, db, "cur-"+string(rune('a'+i)), now.Add(-2*24*time.Hour))
	}
	// Rows inside the prior 7-day window [-14d, -7d) (counted in PreviousValue).
	for i := 0; i < 2; i++ {
		insertAnime(t, db, "prev-"+string(rune('a'+i)), now.Add(-10*24*time.Hour))
	}

	c := newFakeCache()
	r := NewPlatformStatsResolver(db, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data, ok := card.Data.(spotlight.PlatformStatsData)
	if !ok {
		t.Fatalf("Card.Data not PlatformStatsData: %T", card.Data)
	}
	if len(data.Metrics) == 0 {
		t.Fatal("expected at least 1 emitted metric")
	}
	for _, m := range data.Metrics {
		if len(m.Series) != 7 {
			t.Errorf("metric %q: len(Series) = %d, want 7", m.Key, len(m.Series))
		}
		if m.PreviousValue == nil {
			t.Errorf("metric %q: PreviousValue is nil, want non-nil", m.Key)
		}
	}

	// Spot-check the numbers for the single Phase-1 metric.
	m := data.Metrics[0]
	if m.PreviousValue == nil || *m.PreviousValue != 2 {
		t.Errorf("PreviousValue = %v, want 2 (prior-window rows)", m.PreviousValue)
	}
	// Series sum over the trailing window must equal Value (3 in-window rows).
	var sum int
	for _, v := range m.Series {
		sum += v
	}
	if sum != int(m.Value) {
		t.Errorf("sum(Series) = %d, want Value = %d", sum, m.Value)
	}
}

func TestPlatformStats_Resolve_NoMetricsComputable_ReturnsNilNil(t *testing.T) {
	// nil db drives the "metric failed" branch — no metrics appended →
	// resolver returns (nil, nil).
	c := newFakeCache()
	r := NewPlatformStatsResolver(nil, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err on no-metrics, got: %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card on no-metrics, got: %+v", card)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set on no-metrics, got %d", c.sets)
	}
}

func TestPlatformStats_Resolve_CacheHit_SkipsDB(t *testing.T) {
	c := newFakeCache()
	seeded := spotlight.PlatformStatsData{Metrics: []spotlight.StatsMetric{
		{Key: "anime_added_7d", Value: 42},
	}}
	key := "spotlight:stats:" + spotlight.DateKeyUTC(time.Now())
	data, _ := json.Marshal(seeded)
	c.store[key] = data

	// Pass nil db deliberately — if cache-hit short-circuits correctly,
	// we never touch the DB and never panic.
	r := NewPlatformStatsResolver(nil, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	got, _ := card.Data.(spotlight.PlatformStatsData)
	if len(got.Metrics) != 1 || got.Metrics[0].Value != 42 {
		t.Errorf("expected cached value 42, got: %+v", got)
	}
}

func TestPlatformStats_Resolve_CacheKeyPrefix(t *testing.T) {
	db := newTestDB(t)
	c := newFakeCache()
	// Insert 1 row so the resolver is eligible and writes the cache.
	insertAnime(t, db, "x", time.Now().Add(-1*24*time.Hour))
	r := NewPlatformStatsResolver(db, c, testLogger())
	_, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	keys := c.keys()
	if len(keys) != 1 || !strings.HasPrefix(keys[0], "spotlight:stats:") {
		t.Errorf("cache key wrong: %v", keys)
	}
}
