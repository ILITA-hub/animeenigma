//go:build integration

package repo

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"testing"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
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
	got, err := r.GetByShikimoriEpisode(context.Background(), "57466", 1, "")
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

	_, err := r.GetByShikimoriEpisode(context.Background(), "doesnotexist", 1, "")
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

// TestEpisodeRepository_ListAdminLegacyPath_FiltersAndRepoints exercises the
// two Phase-7 migrator hooks end-to-end against a real DB: ListAdminLegacyPath
// must return ONLY rows still on the legacy "{id}/{ep}/" prefix (excluding any
// already on aeProvider/), and UpdateMinioPath must repoint a single row so a
// subsequent list no longer returns it (idempotency / restart-safety).
func TestEpisodeRepository_ListAdminLegacyPath_FiltersAndRepoints(t *testing.T) {
	db, cleanup := openEpisodeTestDB(t)
	defer cleanup()
	// 005 adds source/track/ledger columns the migrator filter does not need,
	// but the model maps them, so apply it for column parity. Idempotent.
	if err := db.Exec(migrations.AutocachePoolSQL).Error; err != nil {
		t.Fatalf("apply 005: %v", err)
	}
	r := NewEpisodeRepository(db)
	ctx := context.Background()

	// Two legacy rows + one already-migrated row.
	legacy1 := &domain.Episode{ShikimoriID: "300", EpisodeNumber: 1, MinioPath: "300/1/"}
	legacy2 := &domain.Episode{ShikimoriID: "300", EpisodeNumber: 2, MinioPath: "300/2/"}
	migrated := &domain.Episode{ShikimoriID: "300", EpisodeNumber: 3, MinioPath: "aeProvider/300/RAW/3/"}
	for _, ep := range []*domain.Episode{legacy1, legacy2, migrated} {
		if err := r.Create(ctx, ep); err != nil {
			t.Fatalf("create %s: %v", ep.MinioPath, err)
		}
	}

	got, err := r.ListAdminLegacyPath(ctx)
	if err != nil {
		t.Fatalf("ListAdminLegacyPath: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAdminLegacyPath len = %d, want 2 (already-migrated row must be excluded)", len(got))
	}
	for _, ep := range got {
		if ep.MinioPath == migrated.MinioPath {
			t.Fatalf("aeProvider/ row leaked into legacy list: %s", ep.MinioPath)
		}
	}

	// Repoint one legacy row; it must drop out of the legacy list.
	newPrefix := "aeProvider/300/RAW/1/"
	if err := r.UpdateMinioPath(ctx, legacy1.ID, newPrefix); err != nil {
		t.Fatalf("UpdateMinioPath: %v", err)
	}
	after, err := r.ListAdminLegacyPath(ctx)
	if err != nil {
		t.Fatalf("ListAdminLegacyPath (after repoint): %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("after repoint len = %d, want 1", len(after))
	}
	if after[0].ID != legacy2.ID {
		t.Fatalf("remaining legacy row = %s, want legacy2 (%s)", after[0].ID, legacy2.ID)
	}
	// Confirm the repoint actually persisted to the row.
	reread, err := r.GetByShikimoriEpisode(ctx, "300", 1, "")
	if err != nil {
		t.Fatalf("re-read repointed row: %v", err)
	}
	if reread.MinioPath != newPrefix {
		t.Fatalf("repointed minio_path = %q, want %q", reread.MinioPath, newPrefix)
	}
}

// openFullEpisodeTestDB is like openEpisodeTestDB but additionally applies
// every migration that adds a column the domain.Episode struct maps (005,
// 015, 016, 017) BEFORE the new 017 dual-storage tests run — GORM's Create
// inserts every mapped struct field regardless of zero value, so a DB
// missing any of those columns 42703s on the very first insert. Also asserts
// re-applying 017 is idempotent (mirrors the 002/003 idempotence check in
// openEpisodeTestDB).
func openFullEpisodeTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db, cleanup := openEpisodeTestDB(t)
	for _, sql := range []struct {
		name string
		stmt string
	}{
		{"005", migrations.AutocachePoolSQL},
		{"015", migrations.StoryboardSQL},
		{"016", migrations.EpisodeAudioLangSQL},
		{"017", migrations.EpisodeStorageSQL},
	} {
		if err := db.Exec(sql.stmt).Error; err != nil {
			cleanup()
			t.Fatalf("apply %s: %v", sql.name, err)
		}
	}
	// Re-apply 017 to prove idempotence (ADD COLUMN IF NOT EXISTS + DROP
	// CONSTRAINT IF EXISTS + the DO $$ ... EXCEPTION-guarded constraint add).
	if err := db.Exec(migrations.EpisodeStorageSQL).Error; err != nil {
		cleanup()
		t.Fatalf("re-apply 017 must be idempotent: %v", err)
	}
	return db, cleanup
}

// TestEpisodeRepository_DualStorageUniqueConstraint is the storage-service
// Task-3 TDD anchor: two rows sharing (shikimori_id, episode_number) but
// differing storage ('minio' vs 's3') must BOTH succeed — the whole point of
// migration 017's dual-presence key — while a third row that repeats an
// already-used (shikimori_id, episode_number, storage) triple must be
// rejected AlreadyExists, exactly like the pre-017 (shikimori_id,
// episode_number)-only constraint used to reject any duplicate episode.
func TestEpisodeRepository_DualStorageUniqueConstraint(t *testing.T) {
	db, cleanup := openFullEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)
	ctx := context.Background()

	minioEp := &domain.Episode{
		ShikimoriID: "9001", EpisodeNumber: 1,
		MinioPath: "aeProvider/9001/RAW/1/", Storage: "minio",
	}
	if err := r.Create(ctx, minioEp); err != nil {
		t.Fatalf("create minio row: %v", err)
	}

	s3Ep := &domain.Episode{
		ShikimoriID: "9001", EpisodeNumber: 1,
		MinioPath: "aeProvider/9001/RAW/1/", Storage: "s3",
	}
	if err := r.Create(ctx, s3Ep); err != nil {
		t.Fatalf("create s3 row for the SAME (shikimori_id, episode_number) as the minio row must succeed: %v", err)
	}

	dupEp := &domain.Episode{
		ShikimoriID: "9001", EpisodeNumber: 1,
		MinioPath: "aeProvider/9001/RAW/1/dup/", Storage: "minio",
	}
	err := r.Create(ctx, dupEp)
	if err == nil {
		t.Fatal("expected AlreadyExists on duplicate (shikimori_id, episode_number, storage), got nil")
	}
	var appErr *liberrors.AppError
	if !stderrors.As(err, &appErr) || appErr.Code != liberrors.CodeAlreadyExists {
		t.Fatalf("Create dup-storage error = %v, want CodeAlreadyExists", err)
	}

	got, err := r.List(ctx, "9001")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("list len = %d, want 2 (minio + s3 rows, dup rejected)", len(got))
	}
}

// TestEpisodeRepository_GetByShikimoriEpisode_StoragePreference is the Task-4
// storage-preference contract: with two rows for the same (shikimori_id,
// episode_number) differing only by storage, an explicit storage argument pins
// the returned row to that backend, while an empty argument is deterministic —
// it prefers the local 'minio' row.
func TestEpisodeRepository_GetByShikimoriEpisode_StoragePreference(t *testing.T) {
	db, cleanup := openFullEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)
	ctx := context.Background()

	// Insert s3 FIRST so an unordered .First() would surface it — proving the
	// minio-preference ORDER BY (not insertion order) drives the empty-arg pick.
	s3Ep := &domain.Episode{ShikimoriID: "9300", EpisodeNumber: 1, MinioPath: "aeProvider/9300/RAW/1/", Storage: "s3"}
	minioEp := &domain.Episode{ShikimoriID: "9300", EpisodeNumber: 1, MinioPath: "aeProvider/9300/RAW/1/", Storage: "minio"}
	for _, ep := range []*domain.Episode{s3Ep, minioEp} {
		if err := r.Create(ctx, ep); err != nil {
			t.Fatalf("create %s: %v", ep.Storage, err)
		}
	}

	// Explicit storage pins the backend.
	gotS3, err := r.GetByShikimoriEpisode(ctx, "9300", 1, "s3")
	if err != nil {
		t.Fatalf("get s3: %v", err)
	}
	if gotS3.Storage != "s3" {
		t.Fatalf("explicit ?storage=s3 returned storage=%q, want s3", gotS3.Storage)
	}

	// Empty arg → minio-first deterministic preference.
	gotDefault, err := r.GetByShikimoriEpisode(ctx, "9300", 1, "")
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if gotDefault.Storage != "minio" {
		t.Fatalf("empty storage arg returned storage=%q, want minio (minio-first preference)", gotDefault.Storage)
	}
}

// TestEpisodeRepository_EvictorQueries_ExcludeS3Rows is the DB-backed half of
// the source tripwire in episode_test.go: an s3-storage row must never
// surface from SumPoolBytes, ListStaleEvictionCandidates, or ListPool — the
// Evictor frees LOCAL disk and must never touch/count a row it can't delete
// local bytes for.
func TestEpisodeRepository_EvictorQueries_ExcludeS3Rows(t *testing.T) {
	db, cleanup := openFullEpisodeTestDB(t)
	defer cleanup()
	r := NewEpisodeRepository(db)
	ctx := context.Background()

	oldTime := time.Now().AddDate(0, -2, 0) // well outside any Fresh window
	size := int64(1000)
	minioEp := &domain.Episode{
		ShikimoriID: "9100", EpisodeNumber: 1,
		MinioPath: "aeProvider/9100/RAW/1/", Storage: "minio",
		Source: domain.EpisodeSourceAdmin, SizeBytes: &size,
		DownloadedAt: &oldTime,
	}
	s3Ep := &domain.Episode{
		ShikimoriID: "9100", EpisodeNumber: 2,
		MinioPath: "aeProvider/9100/RAW/2/", Storage: "s3",
		Source: domain.EpisodeSourceAdmin, SizeBytes: &size,
		DownloadedAt: &oldTime,
	}
	for _, ep := range []*domain.Episode{minioEp, s3Ep} {
		if err := r.Create(ctx, ep); err != nil {
			t.Fatalf("create %+v: %v", ep, err)
		}
	}

	// SumPoolBytes: only the minio row's 1000 bytes should count.
	total, err := r.SumPoolBytes(ctx)
	if err != nil {
		t.Fatalf("SumPoolBytes: %v", err)
	}
	if total != 1000 {
		t.Fatalf("SumPoolBytes = %d, want 1000 (s3 row must be excluded)", total)
	}

	// ListPool: only the minio row.
	pool, err := r.ListPool(ctx)
	if err != nil {
		t.Fatalf("ListPool: %v", err)
	}
	if len(pool) != 1 || pool[0].ID != minioEp.ID {
		t.Fatalf("ListPool = %v, want only the minio row (%s)", ids(pool), minioEp.ID)
	}

	// ListStaleEvictionCandidates: both rows are Stale (2 months old, well past
	// any default Fresh window), but only the minio row may be a candidate.
	cfg := &domain.AutocacheConfig{
		AutoFreshDownloadDays: 1, AutoFreshFetchDays: 1, AdminFreshDays: 1,
		BudgetBytes: 1,
	}
	cands, err := r.ListStaleEvictionCandidates(ctx, cfg, time.Now())
	if err != nil {
		t.Fatalf("ListStaleEvictionCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != minioEp.ID {
		t.Fatalf("ListStaleEvictionCandidates = %v, want only the minio row (%s)", ids(cands), minioEp.ID)
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
