package repo

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqliteOnce ensures the custom SQLite driver is registered exactly once per
// test binary (sql.Register panics on duplicate name).
var sqliteOnce sync.Once

// genRandomUUID returns a random UUID v4 string, used as the SQLite
// substitute for Postgres's gen_random_uuid() builtin.
func genRandomUUID() string {
	b := make([]byte, 16)
	rand.Read(b)                //nolint:gosec // test-only, non-cryptographic use
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func registerSQLiteNow() {
	sqliteOnce.Do(func() {
		sql.Register("sqlite3_with_now", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true); err != nil {
					return err
				}
				// SQLite does not have gen_random_uuid(); register a Go
				// implementation so AutoMigrate can create the upscale_jobs
				// table (which uses it as the id column default).
				return conn.RegisterFunc("gen_random_uuid", genRandomUUID, false)
			},
		})
	})
}

// openTestDB opens a shared in-memory SQLite database with all four upscaler
// domain models migrated, and registers a t.Cleanup to truncate tables so tests
// are isolated from each other within the same process.
//
// SQLite caveat: the domain models use `DEFAULT gen_random_uuid()` which is a
// Postgres builtin not understood by SQLite's DDL parser. We therefore create
// the tables with hand-written SQLite-compatible DDL instead of AutoMigrate.
// GORM operations (Find/Create/Updates/etc.) work fine once the tables exist.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{
		DriverName: "sqlite3_with_now",
		DSN:        "file:upscaler_test?mode=memory&cache=shared",
	}, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}

	// Create tables using SQLite-compatible DDL (gen_random_uuid() is Postgres-only;
	// tests supply IDs directly so there is no need for a SQL-level default).
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS upscale_jobs (
			id                TEXT NOT NULL PRIMARY KEY,
			shikimori_id      TEXT NOT NULL,
			episode           INTEGER NOT NULL,
			library_infohash  TEXT,
			model             TEXT NOT NULL,
			scale             INTEGER NOT NULL DEFAULT 2,
			status            TEXT NOT NULL DEFAULT 'queued',
			progress_pct      INTEGER NOT NULL DEFAULT 0,
			source_codec      TEXT,
			source_pixfmt     TEXT,
			source_fps        TEXT,
			source_height     INTEGER NOT NULL DEFAULT 0,
			segment_count     INTEGER NOT NULL DEFAULT 0,
			output_prefix     TEXT,
			storage           TEXT NOT NULL DEFAULT '',
			error_text        TEXT,
			created_at        DATETIME,
			updated_at        DATETIME,
			completed_at      DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upscale_jobs_shikimori_id ON upscale_jobs(shikimori_id)`,
		`CREATE INDEX IF NOT EXISTS idx_upscale_jobs_status ON upscale_jobs(status)`,
		`CREATE TABLE IF NOT EXISTS upscale_segments (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upscale_segments_status ON upscale_segments(status)`,
		`CREATE TABLE IF NOT EXISTS upscale_workers (
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
		)`,
		`CREATE TABLE IF NOT EXISTS upscale_models (
			name        TEXT NOT NULL,
			version     TEXT NOT NULL,
			checksum    TEXT NOT NULL,
			object_path TEXT NOT NULL,
			builtin     INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME,
			PRIMARY KEY (name, version)
		)`,
	}
	for _, ddl := range ddls {
		if err := db.Exec(ddl).Error; err != nil {
			t.Skipf("create table: %v", err)
		}
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM upscale_segments")
		db.Exec("DELETE FROM upscale_jobs")
		db.Exec("DELETE FROM upscale_workers")
		db.Exec("DELETE FROM upscale_models")
	})
	return db
}

// makeJob is a helper that inserts a minimal UpscaleJob and returns its ID.
// UUID is generated in Go because SQLite has no gen_random_uuid() builtin.
func makeJob(t *testing.T, db *gorm.DB) string {
	t.Helper()
	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "12345",
		Episode:     1,
		Model:       "realesrgan-x4plus-anime",
		Scale:       4,
		Status:      domain.JobQueued,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("makeJob: %v", err)
	}
	return job.ID
}

// Test_SegmentLeaseLedger covers the core segment lifecycle: bulk insert →
// lease → mark done → expiry flip-back-to-pending.
func Test_SegmentLeaseLedger(t *testing.T) {
	db := openTestDB(t)
	sr := NewSegmentRepository(db)
	ctx := context.Background()

	jobID := makeJob(t, db)
	const ttl = 10 * time.Second

	t.Run("BulkInsertPending creates n pending segments", func(t *testing.T) {
		if err := sr.BulkInsertPending(ctx, jobID, 3); err != nil {
			t.Fatalf("BulkInsertPending: %v", err)
		}
		pending, leased, done, err := sr.Counts(ctx, jobID)
		if err != nil {
			t.Fatalf("Counts: %v", err)
		}
		if pending != 3 || leased != 0 || done != 0 {
			t.Fatalf("Counts after BulkInsert = (%d,%d,%d), want (3,0,0)", pending, leased, done)
		}
	})

	t.Run("LeaseNext twice yields two distinct segments, both leased", func(t *testing.T) {
		seg0, err := sr.LeaseNext(ctx, jobID, "worker-A", ttl)
		if err != nil {
			t.Fatalf("LeaseNext #1: %v", err)
		}
		if seg0 == nil {
			t.Fatal("LeaseNext #1 returned nil, want a segment")
		}

		seg1, err := sr.LeaseNext(ctx, jobID, "worker-B", ttl)
		if err != nil {
			t.Fatalf("LeaseNext #2: %v", err)
		}
		if seg1 == nil {
			t.Fatal("LeaseNext #2 returned nil, want a second segment")
		}

		if seg0.Idx == seg1.Idx {
			t.Fatalf("both LeaseNext calls returned idx=%d, want distinct idx", seg0.Idx)
		}

		pending, leased, done, err := sr.Counts(ctx, jobID)
		if err != nil {
			t.Fatalf("Counts: %v", err)
		}
		if pending != 1 || leased != 2 || done != 0 {
			t.Fatalf("Counts after 2 leases = (%d,%d,%d), want (1,2,0)", pending, leased, done)
		}
	})

	t.Run("MarkDone transitions one leased segment to done", func(t *testing.T) {
		// idx 0 was leased first
		if err := sr.MarkDone(ctx, jobID, 0, 1024); err != nil {
			t.Fatalf("MarkDone idx=0: %v", err)
		}
		pending, leased, done, err := sr.Counts(ctx, jobID)
		if err != nil {
			t.Fatalf("Counts: %v", err)
		}
		if done != 1 {
			t.Fatalf("done count = %d, want 1 after MarkDone", done)
		}
		// Check out_bytes persisted.
		segs, err := sr.ListByJob(ctx, jobID)
		if err != nil {
			t.Fatalf("ListByJob: %v", err)
		}
		for _, s := range segs {
			if s.Idx == 0 && s.OutBytes != 1024 {
				t.Fatalf("seg idx=0 out_bytes=%d, want 1024", s.OutBytes)
			}
		}
		_ = pending
		_ = leased
	})

	t.Run("ExpireStale flips leased-past-deadline back to pending", func(t *testing.T) {
		// All leased segments have lease_expires_at ≈ now+10s; passing now+ttl+1s
		// should expire them.
		future := time.Now().Add(ttl + 1*time.Second)
		n, err := sr.ExpireStale(ctx, future)
		if err != nil {
			t.Fatalf("ExpireStale: %v", err)
		}
		if n < 1 {
			t.Fatalf("ExpireStale flipped %d segments, want ≥1 (the still-leased one)", n)
		}
		pending, leased, done, err := sr.Counts(ctx, jobID)
		if err != nil {
			t.Fatalf("Counts after ExpireStale: %v", err)
		}
		if leased != 0 {
			t.Fatalf("leased count = %d after ExpireStale, want 0", leased)
		}
		_ = pending
		_ = done
	})
}
