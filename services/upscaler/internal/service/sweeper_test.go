package service

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestMain initialises the capability secret once for all service tests.
func TestMain(m *testing.M) {
	capability.Init("svc-test-secret")
	os.Exit(m.Run())
}

// ── SQLite test-DB helper ─────────────────────────────────────────────────────

var svcSQLiteOnce sync.Once

func svcGenRandomUUID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:gosec // test-only, non-cryptographic use
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func registerSvcSQLiteDriver() {
	svcSQLiteOnce.Do(func() {
		sql.Register("sqlite3_with_now_svc", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true); err != nil {
					return err
				}
				return conn.RegisterFunc("gen_random_uuid", svcGenRandomUUID, false)
			},
		})
	})
}

const segmentsDDL = `CREATE TABLE IF NOT EXISTS upscale_segments (
	job_id           TEXT NOT NULL,
	idx              INTEGER NOT NULL,
	status           TEXT NOT NULL DEFAULT 'pending',
	lease_expires_at DATETIME,
	worker_id        TEXT,
	in_bytes         INTEGER NOT NULL DEFAULT 0,
	out_bytes        INTEGER NOT NULL DEFAULT 0,
	started_at       DATETIME,
	completed_at     DATETIME,
	PRIMARY KEY (job_id, idx)
)`

const svcWorkersDDL = `CREATE TABLE IF NOT EXISTS upscale_workers (
	worker_id          TEXT NOT NULL PRIMARY KEY,
	gpu_info           TEXT,
	image_version      TEXT,
	models_available   TEXT,
	status             TEXT NOT NULL DEFAULT 'idle',
	current_job_id     TEXT,
	current_segment    INTEGER,
	session_expires_at DATETIME,
	last_heartbeat_at  DATETIME,
	created_at         DATETIME
)`

// openSvcTestDB opens a shared in-memory SQLite DB with upscale_segments and
// upscale_workers tables, isolated per test via Cleanup deletions.
func openSvcTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSvcSQLiteDriver()
	db, err := gorm.Open(&sqlite.Dialector{
		DriverName: "sqlite3_with_now_svc",
		DSN:        "file:upscaler_svc_test?mode=memory&cache=shared",
	}, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	for _, ddl := range []string{segmentsDDL, svcWorkersDDL} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Skipf("create table: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM upscale_segments")
		db.Exec("DELETE FROM upscale_workers")
	})
	return db
}

// ── Sweeper tests ─────────────────────────────────────────────────────────────

// TestSweeper_ExpiresStaleLeasesAndGoneWorkers verifies that a single sweeper
// tick:
//  1. Flips a leased-but-expired segment back to pending (clearing worker_id).
//  2. Marks a worker whose last_heartbeat_at is older than staleWorkerThreshold as gone.
//  3. Does NOT touch a segment with status='done'.
func TestSweeper_ExpiresStaleLeasesAndGoneWorkers(t *testing.T) {
	db := openSvcTestDB(t)
	segRepo := repo.NewSegmentRepository(db)
	workerRepo := repo.NewWorkerRepository(db)
	ctx := context.Background()

	jobID := "job-sweep-test"
	workerID := "worker-stale"

	// Seed: one leased segment with lease_expires_at in the past.
	pastExp := time.Now().Add(-5 * time.Minute)
	if err := db.Exec(
		`INSERT INTO upscale_segments (job_id, idx, status, worker_id, lease_expires_at, in_bytes, out_bytes)
		 VALUES (?, 0, 'leased', ?, ?, 0, 0)`,
		jobID, workerID, pastExp,
	).Error; err != nil {
		t.Fatalf("seed stale segment: %v", err)
	}

	// Seed: one done segment — must NOT be touched.
	if err := db.Exec(
		`INSERT INTO upscale_segments (job_id, idx, status, in_bytes, out_bytes)
		 VALUES (?, 1, 'done', 0, 0)`,
		jobID,
	).Error; err != nil {
		t.Fatalf("seed done segment: %v", err)
	}

	// Seed: one stale worker (last heartbeat > staleWorkerThreshold ago).
	staleHB := time.Now().Add(-5 * time.Minute)
	staleWorker := &domain.UpscaleWorker{
		WorkerID:        workerID,
		Status:          "busy",
		LastHeartbeatAt: &staleHB,
	}
	if err := db.Create(staleWorker).Error; err != nil {
		t.Fatalf("seed stale worker: %v", err)
	}

	// Run one sweeper tick directly (no goroutine).
	s := NewSweeper(segRepo, workerRepo)
	s.sweep(ctx)

	// Verify: stale segment flipped to pending.
	var seg domain.UpscaleSegment
	if err := db.Where("job_id = ? AND idx = 0", jobID).First(&seg).Error; err != nil {
		t.Fatalf("fetch segment idx=0: %v", err)
	}
	if seg.Status != domain.SegPending {
		t.Errorf("stale leased segment: status = %q, want %q", seg.Status, domain.SegPending)
	}
	if seg.WorkerID != "" {
		t.Errorf("stale leased segment: worker_id = %q, want empty string", seg.WorkerID)
	}

	// Verify: done segment untouched.
	var done domain.UpscaleSegment
	if err := db.Where("job_id = ? AND idx = 1", jobID).First(&done).Error; err != nil {
		t.Fatalf("fetch segment idx=1: %v", err)
	}
	if done.Status != domain.SegDone {
		t.Errorf("done segment changed: status = %q, want %q", done.Status, domain.SegDone)
	}

	// Verify: stale worker marked gone.
	var w domain.UpscaleWorker
	if err := db.Where("worker_id = ?", workerID).First(&w).Error; err != nil {
		t.Fatalf("fetch worker: %v", err)
	}
	if w.Status != "gone" {
		t.Errorf("stale worker: status = %q, want %q", w.Status, "gone")
	}
}

// TestSweeper_FreshWorkerNotMarkedGone verifies a recently-heartbeating worker
// is NOT marked as gone during a sweep tick.
func TestSweeper_FreshWorkerNotMarkedGone(t *testing.T) {
	db := openSvcTestDB(t)
	segRepo := repo.NewSegmentRepository(db)
	workerRepo := repo.NewWorkerRepository(db)
	ctx := context.Background()

	// Fresh worker: heartbeat 10 seconds ago (well within the 3-minute threshold).
	freshHB := time.Now().Add(-10 * time.Second)
	freshWorker := &domain.UpscaleWorker{
		WorkerID:        "worker-fresh",
		Status:          "busy",
		LastHeartbeatAt: &freshHB,
	}
	if err := db.Create(freshWorker).Error; err != nil {
		t.Fatalf("seed fresh worker: %v", err)
	}

	s := NewSweeper(segRepo, workerRepo)
	s.sweep(ctx)

	var w domain.UpscaleWorker
	if err := db.Where("worker_id = ?", "worker-fresh").First(&w).Error; err != nil {
		t.Fatalf("fetch worker: %v", err)
	}
	if w.Status == "gone" {
		t.Errorf("fresh worker was incorrectly marked gone")
	}
}

// TestSweeper_StopCancelsRun verifies that Stop() terminates the Run() goroutine
// in a reasonable time.
func TestSweeper_StopCancelsRun(t *testing.T) {
	db := openSvcTestDB(t)
	segRepo := repo.NewSegmentRepository(db)
	workerRepo := repo.NewWorkerRepository(db)

	s := NewSweeper(segRepo, workerRepo)

	done := make(chan struct{})
	go func() {
		s.Run(context.Background())
		close(done)
	}()

	s.Stop()

	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper Run() did not return within 2s after Stop()")
	}
}
