//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openTestDBEncoderChain opens a fresh per-test Postgres database and applies
// the library_jobs migration chain the encoder path needs, in main.go order:
//
//	001 (library_jobs table + enums) → 008 (job_source += 'autocache') →
//	009 (episode column — the domain model inserts it) →
//	014 (job_status += 'transcoding' — the in-progress encode state)
//
// The plain openTestDB applies only 001, which is insufficient for the current
// domain model (missing the 009 episode column) — so these tests use their own
// chain. Re-applied twice to prove idempotence. Gated by INTEGRATION=1.
func openTestDBEncoderChain(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run repo integration tests against Postgres")
	}

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")

	dbName := fmt.Sprintf("library_enc_test_%d_%d", os.Getpid(), time.Now().UnixNano())

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

	chain := []string{
		migrations.LibraryJobsSQL,           // 001 — library_jobs + enums
		migrations.AutocacheJobSourceSQL,    // 008 — job_source += 'autocache'
		migrations.LibraryJobsEpisodeSQL,    // 009 — episode column
		migrations.LibraryJobsTranscodingSQL, // 014 — job_status += 'transcoding'
	}
	for pass := 0; pass < 2; pass++ {
		for i, sql := range chain {
			if err := db.Exec(sql).Error; err != nil {
				t.Fatalf("apply migration %d (pass %d): %v", i, pass, err)
			}
		}
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
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

// TestJobRepository_ClaimForEncoding is the double-encode regression. It proves
// that claiming an 'encoding' row flips it to the NON-claimable 'transcoding'
// state, so an immediate second claim — exactly what a second idle encoder
// worker does while the first is mid-ffmpeg — finds nothing. Before the fix the
// row was re-flipped back to claimable 'encoding' for the whole transcode, so a
// second worker re-claimed the SAME row and spawned a second ffmpeg.
func TestJobRepository_ClaimForEncoding(t *testing.T) {
	db, cleanup := openTestDBEncoderChain(t)
	defer cleanup()
	r := NewJobRepository(db)
	ctx := context.Background()

	// One 'encoding' (ready) row, one 'queued', one 'done'. Only the encoding
	// row is eligible for ClaimForEncoding.
	jE := mustInsertJob(t, r, domain.JobSourceManual, "e", "magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=e")
	if err := r.UpdateStatus(ctx, jE.ID, domain.JobStatusEncoding, ""); err != nil {
		t.Fatalf("set encoding: %v", err)
	}
	jQ := mustInsertJob(t, r, domain.JobSourceManual, "q", "magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=q")
	jD := mustInsertJob(t, r, domain.JobSourceManual, "d", "magnet:?xt=urn:btih:3333333333333333333333333333333333333333&dn=d")
	if err := r.UpdateStatus(ctx, jD.ID, domain.JobStatusDone, ""); err != nil {
		t.Fatalf("set done: %v", err)
	}

	// First claim: gets the encoding row, returned already as transcoding.
	got, err := r.ClaimForEncoding(ctx)
	if err != nil {
		t.Fatalf("first ClaimForEncoding: %v", err)
	}
	if got == nil {
		t.Fatalf("first ClaimForEncoding returned nil; want the encoding row")
	}
	if got.ID != jE.ID {
		t.Fatalf("claimed id = %s, want %s", got.ID, jE.ID)
	}
	if got.Status != domain.JobStatusTranscoding {
		t.Fatalf("returned status = %q, want transcoding", got.Status)
	}
	// The persisted row must be 'transcoding' too.
	if row, _ := r.GetByID(ctx, jE.ID); row.Status != domain.JobStatusTranscoding {
		t.Fatalf("persisted status = %q, want transcoding", row.Status)
	}

	// Second claim — the regression. The only encoding row is now transcoding
	// (NOT claimable), so a second worker gets nothing instead of re-claiming
	// the same in-flight row.
	again, err := r.ClaimForEncoding(ctx)
	if err != nil {
		t.Fatalf("second ClaimForEncoding: %v", err)
	}
	if again != nil {
		t.Fatalf("second ClaimForEncoding returned id=%s status=%q; want nil (no double-claim of a transcoding row)", again.ID, again.Status)
	}

	// Queued / done rows are untouched.
	if row, _ := r.GetByID(ctx, jQ.ID); row.Status != domain.JobStatusQueued {
		t.Fatalf("queued row mutated to %q", row.Status)
	}
	if row, _ := r.GetByID(ctx, jD.ID); row.Status != domain.JobStatusDone {
		t.Fatalf("done row mutated to %q", row.Status)
	}
}

// TestJobRepository_ResumeInterruptedEncodes proves the boot hook requeues
// IN-PROGRESS encode rows ('transcoding', 'uploading') back to 'queued' with NO
// staleness guard — a row killed seconds ago (recent updated_at) is resumed,
// which the old `updated_at < now() - 1h` guard wrongly stranded after a
// restart. The 'encoding' (ready) row is left alone (the encoder self-heals it
// via ClaimForEncoding); queued/done rows are untouched.
func TestJobRepository_ResumeInterruptedEncodes(t *testing.T) {
	db, cleanup := openTestDBEncoderChain(t)
	defer cleanup()
	r := NewJobRepository(db)
	ctx := context.Background()

	// All updated_at default to now() (recent) — the regression condition.
	jT := mustInsertJob(t, r, domain.JobSourceManual, "t", "magnet:?xt=urn:btih:4444444444444444444444444444444444444444&dn=t")
	if err := r.UpdateStatus(ctx, jT.ID, domain.JobStatusTranscoding, ""); err != nil {
		t.Fatalf("set transcoding: %v", err)
	}
	jU := mustInsertJob(t, r, domain.JobSourceManual, "u", "magnet:?xt=urn:btih:5555555555555555555555555555555555555555&dn=u")
	if err := r.UpdateStatus(ctx, jU.ID, domain.JobStatusUploading, ""); err != nil {
		t.Fatalf("set uploading: %v", err)
	}
	jE := mustInsertJob(t, r, domain.JobSourceManual, "e", "magnet:?xt=urn:btih:6666666666666666666666666666666666666666&dn=e")
	if err := r.UpdateStatus(ctx, jE.ID, domain.JobStatusEncoding, ""); err != nil {
		t.Fatalf("set encoding: %v", err)
	}
	jQ := mustInsertJob(t, r, domain.JobSourceManual, "q", "magnet:?xt=urn:btih:7777777777777777777777777777777777777777&dn=q")
	jD := mustInsertJob(t, r, domain.JobSourceManual, "d", "magnet:?xt=urn:btih:8888888888888888888888888888888888888888&dn=d")
	if err := r.UpdateStatus(ctx, jD.ID, domain.JobStatusDone, ""); err != nil {
		t.Fatalf("set done: %v", err)
	}

	n, err := r.ResumeInterruptedEncodes(ctx)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if n != 2 {
		t.Fatalf("rows affected = %d, want 2 (transcoding + uploading, recent updated_at, no 1h guard)", n)
	}

	// transcoding + uploading → queued.
	if row, _ := r.GetByID(ctx, jT.ID); row.Status != domain.JobStatusQueued {
		t.Fatalf("transcoding row → %q, want queued", row.Status)
	}
	if row, _ := r.GetByID(ctx, jU.ID); row.Status != domain.JobStatusQueued {
		t.Fatalf("uploading row → %q, want queued", row.Status)
	}
	// 'encoding' (ready) is NOT touched — the encoder self-heals it.
	if row, _ := r.GetByID(ctx, jE.ID); row.Status != domain.JobStatusEncoding {
		t.Fatalf("encoding row → %q, want encoding (left for ClaimForEncoding)", row.Status)
	}
	// queued + done untouched.
	if row, _ := r.GetByID(ctx, jQ.ID); row.Status != domain.JobStatusQueued {
		t.Fatalf("queued row → %q, want queued", row.Status)
	}
	if row, _ := r.GetByID(ctx, jD.ID); row.Status != domain.JobStatusDone {
		t.Fatalf("done row → %q, want done", row.Status)
	}
}
