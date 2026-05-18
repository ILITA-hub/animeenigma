//go:build integration

package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestWorkerPool_TwoWorkersClaimTwoJobs is the Phase-3 must-have
// acceptance criterion: two workers each claim a different queued
// job via FOR UPDATE SKIP LOCKED. We use the REAL JobRepository
// (not the fake one in download_worker_test.go) against a real
// Postgres so the lock semantics actually apply.
//
// We stub the TorrentAdder so the test doesn't talk to the network;
// the handle simply parks and never fires Done(), so both workers
// stay in their tick loop. The assertion is purely that the two
// queued rows transition to 'downloading' with distinct IDs.
func TestWorkerPool_TwoWorkersClaimTwoJobs(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run worker integration test")
	}

	db, cleanup := openIntegrationDB(t)
	defer cleanup()

	jobRepo := repo.NewJobRepository(db)
	// Two queued rows; concrete identical magnets are fine — we
	// stub the adder.
	for i := 0; i < 2; i++ {
		j := &domain.Job{
			Source: domain.JobSourceManual,
			Title:  fmt.Sprintf("concurrent-%d", i),
			Magnet: fmt.Sprintf("magnet:?xt=urn:btih:%040d&dn=c%d", i, i),
		}
		if err := jobRepo.Create(context.Background(), j); err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}

	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)
	adder := &parkingAdder{}
	pool := NewWorkerPool(2, jobRepo, adder, m, 30*time.Minute, 100*time.Millisecond, nil)
	pool.pollInterval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Wait until both rows are in downloading (or beyond), or 3s.
	deadline := time.After(3 * time.Second)
	for {
		jobs, err := jobRepo.List(ctx, repo.JobFilter{Statuses: []domain.JobStatus{domain.JobStatusDownloading}})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(jobs) == 2 {
			// Distinct IDs?
			if jobs[0].ID == jobs[1].ID {
				t.Fatalf("two workers claimed the same row: %+v", jobs[0])
			}
			break
		}
		select {
		case <-deadline:
			jobs, _ := jobRepo.List(ctx, repo.JobFilter{})
			t.Fatalf("expected 2 jobs in downloading; got %d. all: %+v", len(jobs), jobs)
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	_ = pool.Stop(context.Background(), 2*time.Second)
}

// parkingAdder returns a handle that never resolves Done() until
// the test ends. Lets workers stay in their tick loop while the
// test inspects DB state.
type parkingAdder struct{}

func (parkingAdder) Add(ctx context.Context, magnet string) (libtorrent.DownloadHandle, error) {
	return &parkingHandle{done: make(chan struct{})}, nil
}

type parkingHandle struct {
	mu        sync.Mutex
	done      chan struct{}
	cancelled bool
}

func (p *parkingHandle) ID() string                          { return "parked" }
func (p *parkingHandle) Progress() (int64, int64, int)       { return 0, 1, 1 }
func (p *parkingHandle) Done() <-chan struct{}               { return p.done }
func (p *parkingHandle) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelled {
		return
	}
	p.cancelled = true
	close(p.done)
}

// openIntegrationDB mirrors the helper in internal/repo so the
// worker test can stand on its own. Uses a per-test database with
// the migration applied twice for idempotence.
func openIntegrationDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")
	dbName := fmt.Sprintf("library_wp_test_%d_%d", os.Getpid(), time.Now().UnixNano())

	adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, pass)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		t.Fatalf("create database: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("test db connect: %v", err)
	}

	if err := db.Exec(migrations.LibraryJobsSQL).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		if err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)).Error; err != nil {
			t.Logf("drop db cleanup: %v", err)
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
