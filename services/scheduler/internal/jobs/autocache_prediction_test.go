package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// avgEpBytesTest is the per-test avg-raw-ep multiplier. Kept small + exact so the
// expected gauge values (count × avg) are easy to assert with no float drift.
const avgEpBytesTest int64 = 1000

// newPredictionTestDB builds the same in-memory sqlite schema the Logic A test
// uses (watch_history × anime_list × animes), so the shared DISTINCT join runs
// identically in tests and Postgres prod.
func newPredictionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE watch_history (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			episode_number INTEGER,
			player TEXT,
			language TEXT,
			watch_type TEXT,
			watched_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			status TEXT,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			shikimori_id TEXT,
			status TEXT,
			episodes_aired INTEGER,
			name_jp TEXT,
			name TEXT,
			name_en TEXT
		)
	`).Error)
	return db
}

// resetPredictionGauge clears the {component} series between cases so each test
// starts from a clean gauge (promauto registers into the shared default registry,
// so the var persists across tests in the package).
func resetPredictionGauge() {
	metrics.AutocachePredictedBytes.Reset()
}

func predictionOngoing(t *testing.T) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.AutocachePredictedBytes.WithLabelValues("ongoing"))
}

func predictionNextep(t *testing.T) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.AutocachePredictedBytes.WithLabelValues("nextep"))
}

// TestAutocachePrediction_OngoingCount seeds two DISTINCT ongoing anime, each with
// an active JP-audio watching watcher in-window, and asserts the ongoing gauge is
// 2 × avgRawEpBytes (the nextep gauge is also 2 here — both rows are ongoing).
func TestAutocachePrediction_OngoingCount(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()
	seedAnimeRow(t, db, "a1", "111", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedAnimeRow(t, db, "a2", "222", "ongoing", 3)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-2*time.Hour))
	seedWatchRow(t, db, "u2", "a2", "ae", "ja")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(2*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(2*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_NextepDropsOngoingClause seeds one ongoing + one RELEASED
// anime, both with an active JP-audio watcher in-window. nextep drops the
// a.status='ongoing' clause, so it counts BOTH (2); ongoing counts only the
// ongoing one (1).
func TestAutocachePrediction_NextepDropsOngoingClause(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()
	seedAnimeRow(t, db, "a1", "111", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedAnimeRow(t, db, "a2", "222", "released", 12)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u2", "a2", "ae", "ja")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(1*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(2*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_FiltersExcludeNonJPAndStale verifies a DUB watcher
// (watch_type='dub' on a non-ae player) and a stale watcher (al.updated_at
// older than the cutoff) are excluded from BOTH counts.
//
// NOTE (L662): the previously-used kodik/ru exclusion case defaulted to
// watch_type='sub' (seedWatchRow), which the corrected predicate
// (watch_type='sub' OR player IN (ae)) actually COUNTS — any sub combo
// carries original Japanese audio regardless of subtitle language. So the
// genuine exclusion is now a dub combo, matching Logic A.
func TestAutocachePrediction_FiltersExcludeNonJPAndStale(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()

	// Qualifying ongoing JP-audio watcher → counted in both.
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	// Dub combo on a non-ae player (kodik/ru/dub) → excluded (no JP audio).
	seedAnimeRow(t, db, "a2", "222", "ongoing", 5)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-1*time.Hour))
	seedWatchRowWT(t, db, "u2", "a2", "kodik", "ru", "dub")

	// Stale (updated_at older than 30d) JP-audio → excluded.
	seedAnimeRow(t, db, "a3", "333", "ongoing", 5)
	seedListRow(t, db, "u3", "a3", "watching", now.AddDate(0, 0, -45))
	seedWatchRow(t, db, "u3", "a3", "ae", "ja")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(1*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(1*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_CountsEnSubMatchingLogicA is the L662 regression: a sub
// combo that is NOT an ae player and NOT language='ja' (kodik/ru/sub and
// english/en/sub) carries original Japanese audio and MUST be counted, matching
// the corrected Logic A predicate (autocache_logic_a.go:129). Before the fix the
// prediction job used the stale `player IN (ae) OR language='ja'` predicate
// which wrongly excluded both, so this test FAILS on the old code (counts==0) and
// passes after the predicate swap.
func TestAutocachePrediction_CountsEnSubMatchingLogicA(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()

	// kodik/ru/sub: not ae, not lang=ja, but watch_type='sub' → JP audio → counted.
	seedAnimeRow(t, db, "a1", "111", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "kodik", "ru")

	// english/en/sub: an EN-sub combo → JP audio → counted.
	seedAnimeRow(t, db, "a2", "222", "ongoing", 3)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-2*time.Hour))
	seedWatchRow(t, db, "u2", "a2", "english", "en")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(2*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(2*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_ExcludesEmptyShikimoriID verifies the IN-01 alignment:
// anime with an empty shikimori_id are EXCLUDED from both counts (mirroring Logic A's
// downstream skip, autocache_logic_a.go:121), instead of collapsing into one distinct
// bucket. Two empty-shikimori_id anime + one real one → counts are 1, not 2 or 3.
func TestAutocachePrediction_ExcludesEmptyShikimoriID(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()

	// Real shikimori_id → counted (Logic A would fire demand for it).
	seedAnimeRow(t, db, "a1", "111", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	// Two DISTINCT anime with EMPTY shikimori_id → both excluded (Logic A skips them).
	// Pre-IN-01 these collapsed into a single DISTINCT '' bucket (count contribution 1);
	// now they contribute 0.
	seedAnimeRow(t, db, "a2", "", "ongoing", 5)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u2", "a2", "ae", "ja")
	seedAnimeRow(t, db, "a3", "", "ongoing", 5)
	seedListRow(t, db, "u3", "a3", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u3", "a3", "ae", "ja")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(1*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(1*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_CountsDistinctByID verifies the count keys on a.id (the
// non-null PK), not a.shikimori_id — so two distinct anime that happen to SHARE a
// shikimori_id (the column is index-only, NOT uniqueIndex) are counted as 2, matching
// Logic A's per-anime (a.id) granularity rather than collapsing under DISTINCT shikimori_id.
func TestAutocachePrediction_CountsDistinctByID(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	now := time.Now()

	// Two distinct ongoing anime sharing the SAME non-empty shikimori_id.
	seedAnimeRow(t, db, "a1", "999", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedAnimeRow(t, db, "a2", "999", "ongoing", 3)
	seedListRow(t, db, "u2", "a2", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u2", "a2", "ae", "ja")

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	// Pre-IN-01 (DISTINCT shikimori_id) this would be 1; now (DISTINCT a.id) it's 2.
	require.Equal(t, float64(2*avgEpBytesTest), predictionOngoing(t))
	require.Equal(t, float64(2*avgEpBytesTest), predictionNextep(t))
}

// TestAutocachePrediction_ZeroRowsSetsZeroAndReturnsNil verifies a clean run with no
// qualifying rows sets BOTH gauges to 0 and returns nil (no error contract).
func TestAutocachePrediction_ZeroRowsSetsZeroAndReturnsNil(t *testing.T) {
	resetPredictionGauge()
	db := newPredictionTestDB(t)
	// Empty tables → zero rows.

	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	require.NoError(t, j.Run(context.Background()))

	require.Equal(t, float64(0), predictionOngoing(t))
	require.Equal(t, float64(0), predictionNextep(t))
}

// TestAutocachePrediction_JoinFailureReturnsError verifies a broken schema (missing
// tables) surfaces as a Run error so the JobService metrics wrap records a real
// failure — Run returns an error ONLY on a query failure.
func TestAutocachePrediction_JoinFailureReturnsError(t *testing.T) {
	resetPredictionGauge()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// No tables created → the counts fail.
	j := NewAutocachePredictionJob(db, 30, avgEpBytesTest, logger.Default())
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("want error on join failure, got nil")
	}
}
