//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// getenv returns the value of env var k, or def when unset/empty.
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// openIntegrationDB provisions a FRESH per-test Postgres database (CREATE
// DATABASE upscaler_it_<pid>_<nanotime>) so concurrent/parallel tests never
// share state — mirrors the per-test-DB pattern in
// services/library/internal/repo/*_integration_test.go. AutoMigrate is applied
// twice to prove idempotence (the second apply must not error). The fresh DB is
// dropped WITH (FORCE) in t.Cleanup.
func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run upscaler integration tests")
	}

	host := getenv("DB_HOST", getenv("PGHOST", "localhost"))
	port := getenv("DB_PORT", getenv("PGPORT", "5432"))
	user := getenv("DB_USER", getenv("PGUSER", "postgres"))
	pass := getenv("DB_PASSWORD", getenv("PGPASSWORD", "postgres"))

	dbName := fmt.Sprintf("upscaler_it_%d_%d", os.Getpid(), time.Now().UnixNano())

	// 1. Connect to the admin (postgres) database to issue CREATE DATABASE.
	adminDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, pass,
	)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("integration postgres unavailable: %v", err)
	}
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}

	// 2. Open a connection to the fresh per-test database.
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect test db %s: %v", dbName, err)
	}

	// 3. AutoMigrate, then re-apply to prove idempotence.
	models := []interface{}{
		&domain.UpscaleJob{},
		&domain.UpscaleSegment{},
		&domain.UpscaleWorker{},
		&domain.UpscaleModel{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("re-apply AutoMigrate must be idempotent: %v", err)
	}

	// 4. Close the connection and DROP the per-test database on cleanup.
	t.Cleanup(func() {
		if sqlDB, _ := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
		if err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)).Error; err != nil {
			t.Logf("drop database %s (cleanup): %v", dbName, err)
		}
		if asqlDB, _ := adminDB.DB(); asqlDB != nil {
			_ = asqlDB.Close()
		}
	})
	return db
}

// TestSegmentRepository_LeaseNext_ConcurrencyExclusion verifies that two
// goroutines racing on LeaseNext for a single-segment job can each claim at
// most one segment, with exactly one winner (non-nil) and one loser (nil).
// This test requires a real Postgres instance (SKIP LOCKED is a no-op on SQLite).
func TestSegmentRepository_LeaseNext_ConcurrencyExclusion(t *testing.T) {
	db := openIntegrationDB(t)
	jr := NewJobRepository(db)
	sr := NewSegmentRepository(db)
	ctx := context.Background()

	// One job with exactly one segment.
	job := &domain.UpscaleJob{
		ShikimoriID: "CONCURRENT_TEST",
		Episode:     1,
		Model:       "realesrgan-x4plus-anime",
		Scale:       4,
		Status:      domain.JobQueued,
	}
	if err := jr.Create(ctx, job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if err := sr.BulkInsertPending(ctx, job.ID, 1); err != nil {
		t.Fatalf("BulkInsertPending: %v", err)
	}

	// Two goroutines race to lease the single segment.
	type result struct {
		seg *domain.UpscaleSegment
		err error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		workerID := fmt.Sprintf("worker-%d", i)
		go func() {
			defer wg.Done()
			seg, err := sr.LeaseNext(ctx, job.ID, workerID, 30*time.Second)
			results[i] = result{seg: seg, err: err}
		}()
	}
	wg.Wait()

	// Validate no errors.
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d got error: %v", i, r.err)
		}
	}

	// Exactly one winner (non-nil), one loser (nil).
	winners := 0
	for _, r := range results {
		if r.seg != nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d (SKIP LOCKED exclusion broken)", winners)
	}

	// After both goroutines: (pending=0, leased=1, done=0).
	pending, leased, done, err := sr.Counts(ctx, job.ID)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if pending != 0 || leased != 1 || done != 0 {
		t.Fatalf("Counts = (%d,%d,%d), want (0,1,0)", pending, leased, done)
	}
}
