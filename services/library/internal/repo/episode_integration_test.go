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

// openEpisodeTestDB creates a per-test database and applies migrations
// 001 + 002 + 003 in order — mirrors the Phase-3 openTestDB helper but
// also runs the new Phase-4 migrations. It also asserts re-applying
// 002 + 003 is idempotent (Phase 4 Acceptance 1).
func openEpisodeTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run episode integration tests")
	}

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")

	dbName := fmt.Sprintf("library_test_ep_%d_%d", os.Getpid(), time.Now().UnixNano())

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

	// Apply 001 (library_jobs — FK target).
	if err := db.Exec(migrations.LibraryJobsSQL).Error; err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	// Apply 002 (library_episodes).
	if err := db.Exec(migrations.LibraryEpisodesSQL).Error; err != nil {
		t.Fatalf("apply 002: %v", err)
	}
	// Apply 003 (library_filename_patterns + seed).
	if err := db.Exec(migrations.LibraryFilenamePatternsSQL).Error; err != nil {
		t.Fatalf("apply 003: %v", err)
	}
	// Re-apply 002 + 003 to prove idempotence.
	if err := db.Exec(migrations.LibraryEpisodesSQL).Error; err != nil {
		t.Fatalf("re-apply 002 must be idempotent: %v", err)
	}
	if err := db.Exec(migrations.LibraryFilenamePatternsSQL).Error; err != nil {
		t.Fatalf("re-apply 003 must be idempotent: %v", err)
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

// TestEpisodeRepository_CreateGetRoundtrip — POST → GET equivalence.
func TestEpisodeRepository_CreateGetRoundtrip(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)

	dur := 1450
	size := int64(987654321)
	ep := &domain.Episode{
		ShikimoriID:   "57466",
		EpisodeNumber: 1,
		MinioPath:     "57466/1/",
		DurationSec:   &dur,
		SizeBytes:     &size,
	}
	if err := r.Create(context.Background(), ep); err != nil {
		t.Fatalf("create: %v", err)
	}
	if ep.ID == "" {
		t.Fatalf("expected server-filled id")
	}
	got, err := r.GetByShikimoriEpisode(context.Background(), "57466", 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ShikimoriID != "57466" || got.EpisodeNumber != 1 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.DurationSec == nil || *got.DurationSec != 1450 {
		t.Fatalf("duration_sec not roundtripped: %+v", got.DurationSec)
	}
	if got.SizeBytes == nil || *got.SizeBytes != 987654321 {
		t.Fatalf("size_bytes not roundtripped: %+v", got.SizeBytes)
	}
}

// TestEpisodeRepository_UniqueConstraint — duplicate (shikimori_id,
// episode_number) is rejected by the DB.
func TestEpisodeRepository_UniqueConstraint(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)

	ep1 := &domain.Episode{ShikimoriID: "100", EpisodeNumber: 1, MinioPath: "100/1/"}
	if err := r.Create(context.Background(), ep1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	ep2 := &domain.Episode{ShikimoriID: "100", EpisodeNumber: 1, MinioPath: "100/1/alt/"}
	err := r.Create(context.Background(), ep2)
	if err == nil {
		t.Fatalf("expected unique-constraint error on duplicate, got nil")
	}
}

// TestEpisodeRepository_NotFound — missing row returns liberrors.NotFound.
func TestEpisodeRepository_NotFound(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)

	_, err := r.GetByShikimoriEpisode(context.Background(), "doesnotexist", 1)
	if err == nil {
		t.Fatalf("expected NotFound, got nil")
	}
}

// TestEpisodeRepository_List orders by episode_number ASC.
func TestEpisodeRepository_List(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)

	// Insert out-of-order.
	for _, n := range []int{3, 1, 2} {
		ep := &domain.Episode{ShikimoriID: "200", EpisodeNumber: n, MinioPath: fmt.Sprintf("200/%d/", n)}
		if err := r.Create(context.Background(), ep); err != nil {
			t.Fatalf("create %d: %v", n, err)
		}
	}
	got, err := r.List(context.Background(), "200")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("list len = %d, want 3", len(got))
	}
	for i, exp := range []int{1, 2, 3} {
		if got[i].EpisodeNumber != exp {
			t.Fatalf("got[%d].EpisodeNumber = %d, want %d (ASC order broken)", i, got[i].EpisodeNumber, exp)
		}
	}
}

// TestFilenamePatternRepository_LoadAll — five SPEC-locked rows seed
// idempotently. Re-applying 003 must not multiply the row count.
func TestFilenamePatternRepository_LoadAll(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	r := NewFilenamePatternRepository(db)

	rows, err := r.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("loadall: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("loadall len = %d, want 5 (seed broken or duplicated)", len(rows))
	}

	want := map[string]bool{
		"Ohys-Raws":    false,
		"SubsPlease":   false,
		"Erai-raws":    false,
		"Leopard-Raws": false,
		"ARC-Raws":     false,
	}
	for _, row := range rows {
		if _, ok := want[row.Uploader]; !ok {
			t.Fatalf("unexpected uploader in seed: %q", row.Uploader)
		}
		want[row.Uploader] = true
		if row.PatternRegex == "" {
			t.Errorf("uploader %s has empty regex", row.Uploader)
		}
	}
	for u, found := range want {
		if !found {
			t.Errorf("expected uploader %s in seed, not found", u)
		}
	}
}
