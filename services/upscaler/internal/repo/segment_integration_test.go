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

func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		host := os.Getenv("PGHOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("PGPORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("PGUSER")
		if user == "" {
			user = "postgres"
		}
		password := os.Getenv("PGPASSWORD")
		dbname := os.Getenv("PGDATABASE")
		if dbname == "" {
			dbname = "upscaler_integration_test"
		}
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbname,
		)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("integration postgres unavailable: %v", err)
	}

	if err := db.AutoMigrate(
		&domain.UpscaleJob{},
		&domain.UpscaleSegment{},
		&domain.UpscaleWorker{},
		&domain.UpscaleModel{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM upscale_segments")
		db.Exec("DELETE FROM upscale_jobs")
		db.Exec("DELETE FROM upscale_workers")
		db.Exec("DELETE FROM upscale_models")
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
