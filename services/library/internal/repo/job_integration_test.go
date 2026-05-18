//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// integrationSchemaSQL is the contents of
// migrations/001_library_jobs.sql, read at test setup time. We don't
// use //go:embed because the migration file lives outside the package
// directory and embed forbids ".." paths.
func loadSchemaSQL(t *testing.T) string {
	t.Helper()
	// services/library/internal/repo/job_integration_test.go →
	// services/library/migrations/001_library_jobs.sql
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := filepath.Join(wd, "..", "..", "migrations", "001_library_jobs.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	return string(b)
}

// openTestDB opens a fresh schema against the local Postgres for the
// duration of one test. The schema is named uniquely per test
// (library_test_<pid>_<nanos>) and DROPPED on cleanup so re-runs don't
// collide.
//
// Gated by `INTEGRATION=1`; otherwise we Skip(). Connection params come
// from DB_HOST / DB_PORT / DB_USER / DB_PASSWORD env vars with
// localhost defaults so it works inside the dev container without
// further config.
func openTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run repo integration tests against Postgres")
	}

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")

	// Create a fresh per-test database. Using a database (not a
	// schema) avoids search_path drift across the pool's connections
	// which would otherwise make concurrent goroutines see the wrong
	// namespace.
	dbName := fmt.Sprintf("library_test_%d_%d", os.Getpid(), time.Now().UnixNano())

	// Connect to the admin database first to CREATE the test DB.
	adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, pass)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres admin: %v", err)
	}
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		t.Fatalf("create database: %v", err)
	}

	// Now open the test database.
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	schemaSQL := loadSchemaSQL(t)
	// Apply the schema SQL. The enum types are scoped to the
	// per-test database, so DO $$ ... EXCEPTION blocks don't fire
	// the first time — but they still must on a re-apply.
	if err := db.Exec(schemaSQL).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	// Re-apply once more to prove idempotence (Acceptance 1).
	if err := db.Exec(schemaSQL).Error; err != nil {
		t.Fatalf("re-apply migration must be idempotent: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		// Drop the per-test database from the admin connection.
		if err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)).Error; err != nil {
			t.Logf("drop database (cleanup): %v", err)
		}
		if asqlDB, _ := adminDB.DB(); asqlDB != nil {
			_ = asqlDB.Close()
		}
	}
	return db, cleanup
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mustInsertJob(t *testing.T, repo *JobRepository, source domain.JobSource, title, magnet string) *domain.Job {
	t.Helper()
	j := &domain.Job{
		Source: source,
		Title:  title,
		Magnet: magnet,
	}
	if err := repo.Create(context.Background(), j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if j.ID == "" {
		t.Fatalf("expected server-filled id, got empty")
	}
	return j
}

// TestJobRepository_CreateGetRoundtrip — POST → GET equivalence.
func TestJobRepository_CreateGetRoundtrip(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	j := mustInsertJob(t, r, domain.JobSourceNyaa, "Roundtrip", "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=rt")
	got, err := r.GetByID(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Title != j.Title || got.Source != j.Source {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, j)
	}
	if got.Status != domain.JobStatusQueued {
		t.Fatalf("default status = %q, want queued", got.Status)
	}
}

// TestJobRepository_ClaimEmpty — empty queue → (nil, nil).
func TestJobRepository_ClaimEmpty(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	got, err := r.Claim(context.Background())
	if err != nil {
		t.Fatalf("Claim on empty queue: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil claim on empty queue, got %+v", got)
	}
}

// TestJobRepository_ClaimFirstQueued — single queued row → returned + flipped to downloading.
func TestJobRepository_ClaimFirstQueued(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	j := mustInsertJob(t, r, domain.JobSourceManual, "first", "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=first")
	claimed, err := r.Claim(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected claim, got nil")
	}
	if claimed.ID != j.ID {
		t.Fatalf("claim wrong row: got %s, want %s", claimed.ID, j.ID)
	}
	if claimed.Status != domain.JobStatusDownloading {
		t.Fatalf("claim did not flip status: got %q, want downloading", claimed.Status)
	}

	// Re-fetch to confirm persistence.
	fresh, err := r.GetByID(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if fresh.Status != domain.JobStatusDownloading {
		t.Fatalf("persisted status = %q, want downloading", fresh.Status)
	}
}

// TestJobRepository_ConcurrentClaim — two parallel Claim() calls
// against two queued rows yield two DIFFERENT rows (FOR UPDATE SKIP
// LOCKED honors the contract). A third concurrent call gets (nil, nil).
//
// This is the Acceptance 4 / Phase-3 must-have. sqlmock cannot honor
// SKIP LOCKED so this MUST run against a real Postgres — and it does,
// under the `integration` build tag with INTEGRATION=1.
func TestJobRepository_ConcurrentClaim(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	jA := mustInsertJob(t, r, domain.JobSourceNyaa, "alpha", "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=a")
	jB := mustInsertJob(t, r, domain.JobSourceAnimeTosho, "bravo", "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&dn=b")

	var wg sync.WaitGroup
	results := make([]*domain.Job, 3)
	errs := make([]error, 3)
	wg.Add(3)
	for i := 0; i < 3; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = r.Claim(context.Background())
		}()
	}
	wg.Wait()

	var claimedIDs []string
	for i, j := range results {
		if errs[i] != nil {
			t.Fatalf("claim[%d] err: %v", i, errs[i])
		}
		if j != nil {
			claimedIDs = append(claimedIDs, j.ID)
		}
	}
	if len(claimedIDs) != 2 {
		t.Fatalf("expected exactly 2 successful claims, got %d (results=%+v)", len(claimedIDs), results)
	}
	if claimedIDs[0] == claimedIDs[1] {
		t.Fatalf("double-claim: both returned %s", claimedIDs[0])
	}
	seen := map[string]bool{jA.ID: false, jB.ID: false}
	for _, id := range claimedIDs {
		if _, ok := seen[id]; !ok {
			t.Fatalf("unexpected claim id %s", id)
		}
		seen[id] = true
	}
	if !seen[jA.ID] || !seen[jB.ID] {
		t.Fatalf("not all rows claimed: %+v", seen)
	}
}

// TestJobRepository_UpdateProgress_Clamps — pct clamps to [0,100] and
// is computed as downloaded * 100 / total.
func TestJobRepository_UpdateProgress_Clamps(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)
	j := mustInsertJob(t, r, domain.JobSourceManual, "p", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=p")

	// 50% case
	if err := r.UpdateProgress(context.Background(), j.ID, 50, 100, 5); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(context.Background(), j.ID)
	if got.ProgressPct != 50 {
		t.Fatalf("pct = %d, want 50", got.ProgressPct)
	}

	// Overshoot → clamp to 100.
	if err := r.UpdateProgress(context.Background(), j.ID, 200, 100, 5); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(context.Background(), j.ID)
	if got.ProgressPct != 100 {
		t.Fatalf("pct overshoot = %d, want 100", got.ProgressPct)
	}

	// Zero total → no-op on pct but updated_at bumped.
	previousUpdated := got.UpdatedAt
	time.Sleep(2 * time.Millisecond)
	if err := r.UpdateProgress(context.Background(), j.ID, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(context.Background(), j.ID)
	if got.ProgressPct != 100 {
		t.Fatalf("pct should not change on zero total; got %d", got.ProgressPct)
	}
	if !got.UpdatedAt.After(previousUpdated) {
		t.Fatalf("updated_at must bump on zero-total tick")
	}
}

// TestJobRepository_UpdateStatus_TerminalSetsCompletedAt.
func TestJobRepository_UpdateStatus_TerminalSetsCompletedAt(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)
	j := mustInsertJob(t, r, domain.JobSourceManual, "s", "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=s")

	if err := r.UpdateStatus(context.Background(), j.ID, domain.JobStatusFailed, "boom"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := r.GetByID(context.Background(), j.ID)
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorText != "boom" {
		t.Fatalf("error_text = %q, want boom", got.ErrorText)
	}
	if got.CompletedAt == nil {
		t.Fatalf("completed_at must be set for terminal status")
	}

	// Non-terminal status doesn't set completed_at.
	j2 := mustInsertJob(t, r, domain.JobSourceManual, "s2", "magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=s2")
	if err := r.UpdateStatus(context.Background(), j2.ID, domain.JobStatusEncoding, ""); err != nil {
		t.Fatal(err)
	}
	got2, _ := r.GetByID(context.Background(), j2.ID)
	if got2.CompletedAt != nil {
		t.Fatalf("completed_at must be nil for non-terminal status")
	}
}

// TestJobRepository_Cancel — queued/downloading flip to cancelled,
// terminal rows are no-op.
func TestJobRepository_Cancel(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	// queued → cancelled
	j1 := mustInsertJob(t, r, domain.JobSourceManual, "c1", "magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=c1")
	if err := r.Cancel(context.Background(), j1.ID); err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	got, _ := r.GetByID(context.Background(), j1.ID)
	if got.Status != domain.JobStatusCancelled {
		t.Fatalf("queued → cancelled failed; got %q", got.Status)
	}

	// downloading → cancelled
	j2 := mustInsertJob(t, r, domain.JobSourceManual, "c2", "magnet:?xt=urn:btih:3333333333333333333333333333333333333333&dn=c2")
	if err := r.UpdateStatus(context.Background(), j2.ID, domain.JobStatusDownloading, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Cancel(context.Background(), j2.ID); err != nil {
		t.Fatalf("cancel downloading: %v", err)
	}
	got, _ = r.GetByID(context.Background(), j2.ID)
	if got.Status != domain.JobStatusCancelled {
		t.Fatalf("downloading → cancelled failed; got %q", got.Status)
	}

	// Cancel on already-done is a no-op (no error, status unchanged).
	j3 := mustInsertJob(t, r, domain.JobSourceManual, "c3", "magnet:?xt=urn:btih:4444444444444444444444444444444444444444&dn=c3")
	if err := r.UpdateStatus(context.Background(), j3.ID, domain.JobStatusDone, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Cancel(context.Background(), j3.ID); err != nil {
		t.Fatalf("cancel done should be no-op, got error: %v", err)
	}
	got, _ = r.GetByID(context.Background(), j3.ID)
	if got.Status != domain.JobStatusDone {
		t.Fatalf("cancel-on-done must not transition; got %q", got.Status)
	}

	// Cancel on missing id → NotFound.
	if err := r.Cancel(context.Background(), "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatalf("cancel on missing id should fail; got nil")
	}
}

// TestJobRepository_ResumeInterruptedDownloads rewrites every
// downloading row back to queued.
func TestJobRepository_ResumeInterruptedDownloads(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	// Three rows: one downloading, one queued, one done. Resume should
	// only touch the downloading row.
	jD := mustInsertJob(t, r, domain.JobSourceManual, "d", "magnet:?xt=urn:btih:5555555555555555555555555555555555555555&dn=d")
	_ = r.UpdateStatus(context.Background(), jD.ID, domain.JobStatusDownloading, "")
	jQ := mustInsertJob(t, r, domain.JobSourceManual, "q", "magnet:?xt=urn:btih:6666666666666666666666666666666666666666&dn=q")
	jX := mustInsertJob(t, r, domain.JobSourceManual, "x", "magnet:?xt=urn:btih:7777777777777777777777777777777777777777&dn=x")
	_ = r.UpdateStatus(context.Background(), jX.ID, domain.JobStatusDone, "")

	n, err := r.ResumeInterruptedDownloads(context.Background())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}
	got, _ := r.GetByID(context.Background(), jD.ID)
	if got.Status != domain.JobStatusQueued {
		t.Fatalf("post-resume status = %q, want queued", got.Status)
	}
	got, _ = r.GetByID(context.Background(), jQ.ID)
	if got.Status != domain.JobStatusQueued {
		t.Fatalf("queued row must remain queued; got %q", got.Status)
	}
	got, _ = r.GetByID(context.Background(), jX.ID)
	if got.Status != domain.JobStatusDone {
		t.Fatalf("done row must remain done; got %q", got.Status)
	}
}
