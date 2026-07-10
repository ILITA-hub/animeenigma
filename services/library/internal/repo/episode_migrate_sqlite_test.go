package repo

import (
	"context"
	"testing"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// mkMigrateEpisode seeds one row with an explicit storage + source so the
// storage-migrate selection + flip queries can be exercised end-to-end.
func mkMigrateEpisode(t *testing.T, db *gorm.DB, id, storage string, source domain.EpisodeSource, createdAt time.Time) {
	t.Helper()
	ep := &domain.Episode{
		ID:            id,
		ShikimoriID:   "s-" + id,
		EpisodeNumber: 1,
		MinioPath:     "aeProvider/" + id + "/RAW/1/",
		Storage:       storage,
		Source:        source,
		Track:         domain.EpisodeTrackRaw,
		CreatedAt:     createdAt,
	}
	if err := db.Create(ep).Error; err != nil {
		t.Fatalf("seed episode %s: %v", id, err)
	}
}

// TestListByStorageSource_SelectsOnlyMinioAutocache asserts the operator's
// selection set: rows on minio with source=autocache, oldest-first, and that
// admin rows and already-flipped s3 rows are excluded (idempotency guarantee).
func TestListByStorageSource_SelectsOnlyMinioAutocache(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	ctx := context.Background()
	now := time.Now()

	// eligible: minio + autocache (older first)
	mkMigrateEpisode(t, db, "auto-old", domain.BackendMinio, domain.EpisodeSourceAutocache, now.Add(-3*time.Hour))
	mkMigrateEpisode(t, db, "auto-new", domain.BackendMinio, domain.EpisodeSourceAutocache, now.Add(-1*time.Hour))
	// excluded: admin content stays local
	mkMigrateEpisode(t, db, "admin1", domain.BackendMinio, domain.EpisodeSourceAdmin, now.Add(-2*time.Hour))
	// excluded: already migrated (flipped to s3) — must never be reselected
	mkMigrateEpisode(t, db, "auto-done", domain.BackendS3, domain.EpisodeSourceAutocache, now.Add(-4*time.Hour))

	got, err := r.ListByStorageSource(ctx, domain.BackendMinio, domain.EpisodeSourceAutocache)
	if err != nil {
		t.Fatalf("ListByStorageSource: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (auto-old, auto-new); ids=%v", len(got), ids(got))
	}
	if got[0].ID != "auto-old" || got[1].ID != "auto-new" {
		t.Fatalf("order = %v, want [auto-old auto-new] (created_at ASC)", ids(got))
	}
}

// TestUpdateStorage_FlipsRowAndDropsFromSelection verifies the flip and that the
// flipped row immediately disappears from the migratable set (re-run safety).
func TestUpdateStorage_FlipsRowAndDropsFromSelection(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	ctx := context.Background()
	mkMigrateEpisode(t, db, "e1", domain.BackendMinio, domain.EpisodeSourceAutocache, time.Now().Add(-1*time.Hour))

	if err := r.UpdateStorage(ctx, "e1", domain.BackendS3); err != nil {
		t.Fatalf("UpdateStorage: %v", err)
	}
	var got domain.Episode
	if err := db.Where("id = ?", "e1").First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Storage != domain.BackendS3 {
		t.Fatalf("storage = %q after UpdateStorage, want %q", got.Storage, domain.BackendS3)
	}
	remaining, err := r.ListByStorageSource(ctx, domain.BackendMinio, domain.EpisodeSourceAutocache)
	if err != nil {
		t.Fatalf("ListByStorageSource: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("after flip, migratable set = %v, want empty", ids(remaining))
	}
	// Unknown id must surface NotFound — the migrator relies on this to detect
	// a row evicted concurrently (it must NOT go on to delete the local prefix).
	err = r.UpdateStorage(ctx, "nope", domain.BackendS3)
	if err == nil {
		t.Fatal("UpdateStorage(unknown) = nil, want NotFound (vanished-row detection)")
	}
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeNotFound {
		t.Fatalf("UpdateStorage(unknown) = %v, want liberrors NotFound", err)
	}
}

// TestGetByID_FoundAndNotFound covers the migrator's flip-failure re-read
// disambiguator.
func TestGetByID_FoundAndNotFound(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	ctx := context.Background()
	mkMigrateEpisode(t, db, "e9", domain.BackendMinio, domain.EpisodeSourceAutocache, time.Now().Add(-1*time.Hour))

	got, err := r.GetByID(ctx, "e9")
	if err != nil {
		t.Fatalf("GetByID(e9): %v", err)
	}
	if got.ID != "e9" || got.Storage != domain.BackendMinio {
		t.Fatalf("GetByID(e9) = %+v, want id=e9 storage=minio", got)
	}

	_, err = r.GetByID(ctx, "missing")
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeNotFound {
		t.Fatalf("GetByID(missing) = %v, want liberrors NotFound", err)
	}
}
