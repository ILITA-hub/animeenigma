//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// loadSchemaSQL pulls the migration via the embed-backed migrations
// package so we exercise the same code path main.go takes — a typo
// in the embed directive surfaces here too.
func loadSchemaSQL(t *testing.T) string {
	t.Helper()
	return migrations.LibraryJobsSQL
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

// TestJobRepository_SetProgressAndStatus — persists a final progress_pct
// alongside a status transition (clamped to [0,100]); terminal statuses stamp
// completed_at. This is the download worker's only progress write to the DB;
// in-flight samples live in the ProgressStore cache, not here.
func TestJobRepository_SetProgressAndStatus(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)
	j := mustInsertJob(t, r, domain.JobSourceManual, "p", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=p")

	// →encoding at 100% (successful download hand-off). Non-terminal: no completed_at.
	if err := r.SetProgressAndStatus(context.Background(), j.ID, domain.JobStatusEncoding, 100, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(context.Background(), j.ID)
	if got.Status != domain.JobStatusEncoding || got.ProgressPct != 100 {
		t.Fatalf("status/pct = %q/%d, want encoding/100", got.Status, got.ProgressPct)
	}
	if got.CompletedAt != nil {
		t.Fatalf("completed_at must stay nil for a non-terminal status")
	}

	// Overshoot clamps to 100, undershoot clamps to 0.
	if err := r.SetProgressAndStatus(context.Background(), j.ID, domain.JobStatusEncoding, 250, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(context.Background(), j.ID)
	if got.ProgressPct != 100 {
		t.Fatalf("pct overshoot = %d, want clamp 100", got.ProgressPct)
	}

	// →failed at a non-100 pct (where the download died) stamps completed_at
	// and records the error text.
	if err := r.SetProgressAndStatus(context.Background(), j.ID, domain.JobStatusFailed, 42, "stalled"); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(context.Background(), j.ID)
	if got.Status != domain.JobStatusFailed || got.ProgressPct != 42 || got.ErrorText != "stalled" {
		t.Fatalf("failed row = %q/%d/%q, want failed/42/stalled", got.Status, got.ProgressPct, got.ErrorText)
	}
	if got.CompletedAt == nil {
		t.Fatalf("completed_at must be stamped for a terminal status")
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

// TestJobRepository_UpdateShikimoriID_Updates pins the column and
// returns NotFound for missing ids (Phase-5).
func TestJobRepository_UpdateShikimoriID_Updates(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	j := mustInsertJob(t, r, domain.JobSourceManual, "u", "magnet:?xt=urn:btih:8888888888888888888888888888888888888888&dn=u")

	if err := r.UpdateShikimoriID(context.Background(), j.ID, "57466"); err != nil {
		t.Fatalf("update shikimori_id: %v", err)
	}
	got, _ := r.GetByID(context.Background(), j.ID)
	if got.ShikimoriID != "57466" {
		t.Fatalf("shikimori_id = %q, want 57466", got.ShikimoriID)
	}

	// Missing id → NotFound.
	if err := r.UpdateShikimoriID(context.Background(), "00000000-0000-0000-0000-000000000000", "1"); err == nil {
		t.Fatalf("expected NotFound on missing id")
	}
}

// TestJobRepository_Retry inherits all fields from the failed row,
// sets status=queued, error_text='retry of <oldID>'.
func TestJobRepository_Retry(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	// Create + mark failed. Carry a non-null Episode so the CR-01 preservation
	// assertion below exercises the autocache-row path.
	retryEpisode := 7
	j := &domain.Job{
		Source:      domain.JobSourceAutocache,
		Magnet:      "magnet:?xt=urn:btih:9999999999999999999999999999999999999999&dn=r",
		Title:       "retry-me",
		Uploader:    "Ohys-Raws",
		Quality:     "1080p",
		SizeBytes:   1024,
		ShikimoriID: "12345",
		Episode:     &retryEpisode,
	}
	if err := r.Create(context.Background(), j); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.UpdateStatus(context.Background(), j.ID, domain.JobStatusFailed, "boom"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	fresh, err := r.Retry(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if fresh == nil {
		t.Fatalf("retry returned nil row")
	}
	if fresh.ID == j.ID {
		t.Fatalf("retry must allocate a NEW id, got same %q", fresh.ID)
	}
	if fresh.Status != domain.JobStatusQueued {
		t.Fatalf("retry status = %q, want queued", fresh.Status)
	}
	if fresh.Magnet != j.Magnet || fresh.Title != j.Title || fresh.Uploader != j.Uploader ||
		fresh.Quality != j.Quality || fresh.SizeBytes != j.SizeBytes || fresh.ShikimoriID != j.ShikimoriID ||
		fresh.Source != j.Source {
		t.Fatalf("retry did not inherit fields: %+v", fresh)
	}
	wantErr := "retry of " + j.ID
	if fresh.ErrorText != wantErr {
		t.Fatalf("retry error_text = %q, want %q", fresh.ErrorText, wantErr)
	}
	// CR-01: a retried autocache row MUST preserve the intended Episode so the
	// Planner's HasActiveForEpisode(shikimori_id, episode) single-flight dedup
	// still matches the re-enqueued row (else a duplicate download is enqueued).
	if fresh.Episode == nil {
		t.Fatal("retry dropped Episode (CR-01): autocache retry → episode=NULL breaks TRIG-04 single-flight dedup")
	}
	if *fresh.Episode != retryEpisode {
		t.Fatalf("retry Episode = %d, want %d", *fresh.Episode, retryEpisode)
	}
	// Re-fetch from the DB to confirm the column actually persisted (not just
	// set on the in-memory struct GORM returned).
	persisted, err := r.GetByID(context.Background(), fresh.ID)
	if err != nil {
		t.Fatalf("re-fetch retry row: %v", err)
	}
	if persisted.Episode == nil || *persisted.Episode != retryEpisode {
		t.Fatalf("persisted retry Episode = %v, want %d", persisted.Episode, retryEpisode)
	}

	// Old row still failed.
	old, _ := r.GetByID(context.Background(), j.ID)
	if old.Status != domain.JobStatusFailed {
		t.Fatalf("old row status changed: %q, want failed (audit trail)", old.Status)
	}

	// Retry on a non-failed row → InvalidInput.
	if _, err := r.Retry(context.Background(), fresh.ID); err == nil {
		t.Fatalf("retry on queued must error")
	}

	// Retry on missing id → NotFound.
	if _, err := r.Retry(context.Background(), "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatalf("retry on missing id must error")
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

// mustInsertSizedJob inserts a job with an explicit source/status/size_bytes so the
// SumInflightJobBytes filter can be exercised across the (source, status) matrix.
func mustInsertSizedJob(t *testing.T, r *JobRepository, source domain.JobSource, status domain.JobStatus, size int64, dn string) *domain.Job {
	t.Helper()
	j := &domain.Job{
		Source:    source,
		Title:     dn,
		Magnet:    "magnet:?xt=urn:btih:8888888888888888888888888888888888888888&dn=" + dn,
		SizeBytes: size,
	}
	if err := r.Create(context.Background(), j); err != nil {
		t.Fatalf("create sized job: %v", err)
	}
	if status != domain.JobStatusQueued {
		if err := r.UpdateStatus(context.Background(), j.ID, status, ""); err != nil {
			t.Fatalf("set status %q: %v", status, err)
		}
	}
	return j
}

// TestJobRepository_SumInflightJobBytes (WR-01): only NON-terminal autocache jobs count
// toward the in-flight reservation — terminal autocache rows and non-autocache rows are
// excluded, and an empty/all-terminal set returns 0 (COALESCE NULL→0).
func TestJobRepository_SumInflightJobBytes(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	r := NewJobRepository(db)

	// Empty → 0.
	if got, err := r.SumInflightJobBytes(context.Background()); err != nil || got != 0 {
		t.Fatalf("empty SumInflightJobBytes = (%d, %v), want (0, nil)", got, err)
	}

	// Non-terminal autocache rows COUNT: queued(100) + downloading(200) + encoding(300)
	// + uploading(400) = 1000.
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusQueued, 100, "ac-q")
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusDownloading, 200, "ac-d")
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusEncoding, 300, "ac-e")
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusUploading, 400, "ac-u")
	// Terminal autocache rows are EXCLUDED (the reservation self-releases on done/
	// failed/cancelled — done now counts in SumPoolBytes, the others never will).
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusDone, 1000, "ac-done")
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusFailed, 1000, "ac-fail")
	mustInsertSizedJob(t, r, domain.JobSourceAutocache, domain.JobStatusCancelled, 1000, "ac-cancel")
	// Non-autocache rows are EXCLUDED regardless of status (admin uploads reserve via
	// the handler's synchronous path, not this counter).
	mustInsertSizedJob(t, r, domain.JobSourceManual, domain.JobStatusDownloading, 5000, "manual-d")

	got, err := r.SumInflightJobBytes(context.Background())
	if err != nil {
		t.Fatalf("SumInflightJobBytes: %v", err)
	}
	if got != 1000 {
		t.Fatalf("SumInflightJobBytes = %d, want 1000 (only non-terminal autocache rows)", got)
	}
}
