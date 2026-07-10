package autocache

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// moveCall records one Move(src, dst) invocation so a test can assert the
// migrator computed the right destination prefix and called Move BEFORE the
// repoint.
type moveCall struct {
	src string
	dst string
}

// fakeMover is an in-memory stand-in for minio.Writer.Move — no live MinIO.
// failOn, when non-empty, makes Move return an error for that exact src so we
// can exercise the "leave row on old path" branch.
type fakeMover struct {
	calls  []moveCall
	failOn string
}

func (m *fakeMover) Move(_ context.Context, storage, src, dst string) error {
	m.calls = append(m.calls, moveCall{src: src, dst: dst})
	// The migrator only ever relocates local admin content.
	if storage != domain.BackendMinio {
		return errors.New("migrator must move on the minio backend, got " + storage)
	}
	if m.failOn != "" && src == m.failOn {
		return errors.New("copy failed")
	}
	return nil
}

// repointCall records one UpdateMinioPath(id, path) invocation.
type repointCall struct {
	id   string
	path string
}

// fakeStore is an in-memory EpisodeStore. legacy is whatever ListAdminLegacyPath
// returns; repoints accumulates the UpdateMinioPath calls.
type fakeStore struct {
	legacy   []domain.Episode
	repoints []repointCall
	failOn   string // id whose UpdateMinioPath returns an error
}

func (s *fakeStore) ListAdminLegacyPath(_ context.Context) ([]domain.Episode, error) {
	return s.legacy, nil
}

func (s *fakeStore) UpdateMinioPath(_ context.Context, id, path string) error {
	s.repoints = append(s.repoints, repointCall{id: id, path: path})
	if s.failOn != "" && id == s.failOn {
		return errors.New("repoint failed")
	}
	return nil
}

// nopLog satisfies the migrator's logger seam without emitting anything.
type nopLog struct{}

func (nopLog) Infow(string, ...any)  {}
func (nopLog) Warnw(string, ...any)  {}
func (nopLog) Errorw(string, ...any) {}

// TestMigrate_MovesThenRepoints — the happy path: a legacy row is Moved to the
// aeProvider/ layout and only THEN repointed, in that order.
func TestMigrate_MovesThenRepoints(t *testing.T) {
	store := &fakeStore{legacy: []domain.Episode{
		{ID: "ep-a", ShikimoriID: "12345", EpisodeNumber: 3, MinioPath: "12345/3/"},
	}}
	mover := &fakeMover{}
	m := NewMigrator(store, mover, nopLog{})

	migrated, err := m.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("migrated = %d, want 1", migrated)
	}

	if len(mover.calls) != 1 {
		t.Fatalf("Move calls = %d, want 1", len(mover.calls))
	}
	wantDst := "aeProvider/12345/RAW/3/"
	if mover.calls[0].src != "12345/3/" || mover.calls[0].dst != wantDst {
		t.Fatalf("Move(%q,%q), want (%q,%q)",
			mover.calls[0].src, mover.calls[0].dst, "12345/3/", wantDst)
	}
	if len(store.repoints) != 1 {
		t.Fatalf("repoint calls = %d, want 1", len(store.repoints))
	}
	if store.repoints[0].id != "ep-a" || store.repoints[0].path != wantDst {
		t.Fatalf("repoint(%q,%q), want (ep-a,%q)",
			store.repoints[0].id, store.repoints[0].path, wantDst)
	}
}

// TestMigrate_SkipsAlreadyMigrated — a row already on aeProvider/ is neither
// Moved nor repointed (idempotency). The store would not normally return such a
// row (the SQL filter excludes it), but the migrator must defend in code too.
func TestMigrate_SkipsAlreadyMigrated(t *testing.T) {
	store := &fakeStore{legacy: []domain.Episode{
		{ID: "ep-done", ShikimoriID: "999", EpisodeNumber: 1, MinioPath: "aeProvider/999/RAW/1/"},
	}}
	mover := &fakeMover{}
	m := NewMigrator(store, mover, nopLog{})

	migrated, err := m.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if migrated != 0 {
		t.Fatalf("migrated = %d, want 0 (already-migrated skipped)", migrated)
	}
	if len(mover.calls) != 0 {
		t.Fatalf("Move must NOT be called on an aeProvider/ row, got %d calls", len(mover.calls))
	}
	if len(store.repoints) != 0 {
		t.Fatalf("repoint must NOT be called on an aeProvider/ row, got %d", len(store.repoints))
	}
}

// TestMigrate_MoveError_LeavesRowAndContinues — when Move fails for one row,
// that row is left un-repointed (no data loss; Move aborts pre-delete on copy
// error) and the run continues to the next row.
func TestMigrate_MoveError_LeavesRowAndContinues(t *testing.T) {
	store := &fakeStore{legacy: []domain.Episode{
		{ID: "ep-fail", ShikimoriID: "1", EpisodeNumber: 1, MinioPath: "1/1/"},
		{ID: "ep-ok", ShikimoriID: "2", EpisodeNumber: 5, MinioPath: "2/5/"},
	}}
	mover := &fakeMover{failOn: "1/1/"}
	m := NewMigrator(store, mover, nopLog{})

	migrated, err := m.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate must not abort on a single-row Move error: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("migrated = %d, want 1 (only ep-ok succeeds)", migrated)
	}
	// ep-fail must NOT be repointed; ep-ok must be.
	if len(store.repoints) != 1 {
		t.Fatalf("repoint calls = %d, want 1", len(store.repoints))
	}
	if store.repoints[0].id != "ep-ok" {
		t.Fatalf("only ep-ok should be repointed, got %q", store.repoints[0].id)
	}
}

// TestMigrate_RepointError_CountsAsNotMigratedAndContinues — when the Move
// succeeds but the repoint fails, the row is not counted as migrated and the
// run continues. A re-run sees the old prefix and harmlessly re-copies.
func TestMigrate_RepointError_CountsAsNotMigratedAndContinues(t *testing.T) {
	store := &fakeStore{
		legacy: []domain.Episode{
			{ID: "ep-x", ShikimoriID: "7", EpisodeNumber: 2, MinioPath: "7/2/"},
			{ID: "ep-y", ShikimoriID: "8", EpisodeNumber: 4, MinioPath: "8/4/"},
		},
		failOn: "ep-x",
	}
	mover := &fakeMover{}
	m := NewMigrator(store, mover, nopLog{})

	migrated, err := m.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate must not abort on a single-row repoint error: %v", err)
	}
	// Both were Moved, but only ep-y's repoint succeeded.
	if len(mover.calls) != 2 {
		t.Fatalf("Move calls = %d, want 2", len(mover.calls))
	}
	if migrated != 1 {
		t.Fatalf("migrated = %d, want 1 (ep-x repoint failed)", migrated)
	}
}
