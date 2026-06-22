//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openTestDBWithJobIndexes opens a fresh per-test Postgres database and applies
// the library_jobs migration chain that the index-finding (L578) touches, in the
// same order main.go does:
//
//	001 (library_jobs table + enums) → 008 (job_source += 'autocache') →
//	009 (episode column) → 013 (partial indexes)
//
// 008 is required before 013 because idx_library_jobs_autocache_inflight's partial
// predicate compares source against the 'autocache' enum literal — which only
// exists after 008's ALTER TYPE ... ADD VALUE. main.go applies 008 well before
// 013, so this mirrors the real startup order.
//
// It re-applies the whole chain once more to prove idempotence (the 013 indexes
// use CREATE INDEX IF NOT EXISTS, so a restart must be a safe no-op). Gated by
// INTEGRATION=1 exactly like openTestDB; otherwise the test Skips so the
// no-live-DB CI run stays green.
func openTestDBWithJobIndexes(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run repo integration tests against Postgres")
	}

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")

	dbName := fmt.Sprintf("library_idx_test_%d_%d", os.Getpid(), time.Now().UnixNano())

	adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, pass)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres admin: %v", err)
	}
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		t.Fatalf("create database: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	// Apply the chain twice — the second pass proves every step (incl. the new
	// 013 CREATE INDEX IF NOT EXISTS) is idempotent.
	chain := []string{
		migrations.LibraryJobsSQL,             // 001 — library_jobs + enums
		migrations.AutocacheJobSourceSQL,      // 008 — job_source += 'autocache' (needed by 013's partial predicate)
		migrations.LibraryJobsEpisodeSQL,      // 009 — episode column
		migrations.LibraryJobsEpisodeIndexSQL, // 013 — the partial indexes under test
	}
	for pass := 0; pass < 2; pass++ {
		for i, sql := range chain {
			if err := db.Exec(sql).Error; err != nil {
				t.Fatalf("apply migration %d (pass %d): %v", i, pass, err)
			}
		}
	}

	cleanup := func() {
		if sqlDB, _ := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
		if err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)).Error; err != nil {
			t.Logf("drop database (cleanup): %v", err)
		}
		if asqlDB, _ := adminDB.DB(); asqlDB != nil {
			_ = asqlDB.Close()
		}
	}
	return db, cleanup
}

// indexExists reports whether an index of the given name is present on
// library_jobs (pg_indexes is the per-database catalog view).
func indexExists(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(
		"SELECT count(*) FROM pg_indexes WHERE tablename = 'library_jobs' AND indexname = ?",
		name,
	).Scan(&n).Error; err != nil {
		t.Fatalf("query pg_indexes for %q: %v", name, err)
	}
	return n > 0
}

// TestLibraryJobsEpisodeIndexes (L578) asserts migration 013 created BOTH partial
// indexes that serve the Phase-09 hot queries. Red before the migration file +
// wiring exist (the indexes are absent); green after.
func TestLibraryJobsEpisodeIndexes(t *testing.T) {
	db, cleanup := openTestDBWithJobIndexes(t)
	defer cleanup()

	for _, name := range []string{
		"idx_library_jobs_shikimori_episode",  // serves HasActiveForEpisode
		"idx_library_jobs_autocache_inflight", // serves SumInflightJobBytes
	} {
		if !indexExists(t, db, name) {
			t.Fatalf("expected index %q on library_jobs after migration 013, but it is missing", name)
		}
	}
}

// TestLibraryJobsEpisodeIndexes_AreNonTerminalPartial (L578) asserts both indexes
// are PARTIAL on the non-terminal set so they stay small as the unbounded durable
// library_jobs audit table grows. We read pg_indexes.indexdef and require each
// definition to carry a WHERE predicate excluding all three terminal states —
// catching a regression that drops the partial clause (which would index every
// terminal row too).
func TestLibraryJobsEpisodeIndexes_AreNonTerminalPartial(t *testing.T) {
	db, cleanup := openTestDBWithJobIndexes(t)
	defer cleanup()

	type row struct{ Indexdef string }
	for _, tc := range []struct {
		name        string
		mustContain []string
	}{
		{"idx_library_jobs_shikimori_episode", []string{"WHERE", "shikimori_id", "episode"}},
		{"idx_library_jobs_autocache_inflight", []string{"WHERE", "autocache"}},
	} {
		var r row
		if err := db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE tablename = 'library_jobs' AND indexname = ?",
			tc.name,
		).Scan(&r).Error; err != nil {
			t.Fatalf("read indexdef for %q: %v", tc.name, err)
		}
		if r.Indexdef == "" {
			t.Fatalf("index %q not found", tc.name)
		}
		def := r.Indexdef
		for _, want := range tc.mustContain {
			if !contains(def, want) {
				t.Fatalf("index %q definition %q missing %q (expected a partial index)", tc.name, def, want)
			}
		}
		// All three terminal states must be excluded by the partial predicate.
		for _, term := range []string{"done", "failed", "cancelled"} {
			if !contains(def, term) {
				t.Fatalf("index %q definition %q does not exclude terminal state %q", tc.name, def, term)
			}
		}
	}
}

// TestLibraryJobsEpisodeIndex_UsedByHasActiveForEpisode (L578) seeds the table and
// asserts the planner's HasActiveForEpisode query plan uses an Index Scan on the
// new (shikimori_id, episode) index rather than a Seq Scan. Guards the value of
// the fix — an index that exists but is never chosen is dead weight.
func TestLibraryJobsEpisodeIndex_UsedByHasActiveForEpisode(t *testing.T) {
	db, cleanup := openTestDBWithJobIndexes(t)
	defer cleanup()
	r := NewJobRepository(db)

	// Seed enough non-terminal rows that the planner prefers an index over a
	// seq scan, all keyed to one shikimori_id but distinct episodes.
	const sid = "57466"
	for ep := 0; ep < 500; ep++ {
		// NOTE: keep the magnet literal free of '?' — GORM treats a bare '?' in
		// the SQL string as a positional placeholder and would shift the bind args.
		if err := db.Exec(
			"INSERT INTO library_jobs (source, magnet, title, shikimori_id, episode, status) "+
				"VALUES ('autocache', 'magnet:xt=urn:btih:seed', 'seed', ?, ?, 'queued')",
			sid, ep,
		).Error; err != nil {
			t.Fatalf("seed row ep=%d: %v", ep, err)
		}
	}
	// ANALYZE so the planner has fresh stats.
	if err := db.Exec("ANALYZE library_jobs").Error; err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// Sanity: the repo method still returns the right answer through the index.
	active, err := r.HasActiveForEpisode(context.Background(), sid, 42)
	if err != nil {
		t.Fatalf("HasActiveForEpisode: %v", err)
	}
	if !active {
		t.Fatalf("HasActiveForEpisode(%q, 42) = false, want true (row seeded)", sid)
	}

	var plan []struct {
		QueryPlan string `gorm:"column:QUERY PLAN"`
	}
	if err := db.Raw(
		"EXPLAIN SELECT 1 FROM library_jobs WHERE shikimori_id = ? AND episode = ? "+
			"AND status NOT IN ('done','failed','cancelled') LIMIT 1",
		sid, 42,
	).Scan(&plan).Error; err != nil {
		t.Fatalf("explain: %v", err)
	}
	var planText string
	for _, p := range plan {
		planText += p.QueryPlan + "\n"
	}
	if !contains(planText, "idx_library_jobs_shikimori_episode") {
		t.Fatalf("HasActiveForEpisode plan did not use idx_library_jobs_shikimori_episode:\n%s", planText)
	}
}

// contains is a tiny substring helper (avoids importing strings just for this).
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
