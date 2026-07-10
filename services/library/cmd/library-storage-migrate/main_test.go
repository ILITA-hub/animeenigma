package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// fakeObjectStore implements objectStore with per-(storage,prefix) canned
// listings and records every DeletePrefix call so tests can assert exactly
// what was (not) deleted.
type fakeObjectStore struct {
	// lists maps "storage|prefix" → canned listing. Missing key → empty list.
	lists map[string][]storageclient.Object
	// errors, keyed the same way, override lists.
	listErrs map[string]error

	copyErr error
	copies  []string // "from>to|prefix"

	deleteErr map[string]error // keyed "storage|prefix"
	deletes   []string         // "storage|prefix"
}

func key(storage, prefix string) string { return storage + "|" + prefix }

func (f *fakeObjectStore) List(_ context.Context, storage, prefix string) ([]storageclient.Object, error) {
	if err := f.listErrs[key(storage, prefix)]; err != nil {
		return nil, err
	}
	return f.lists[key(storage, prefix)], nil
}

func (f *fakeObjectStore) CopyPrefix(_ context.Context, from, to, prefix string) (int, int64, error) {
	f.copies = append(f.copies, from+">"+to+"|"+prefix)
	if f.copyErr != nil {
		return 0, 0, f.copyErr
	}
	objs := f.lists[key(to, prefix)]
	c, b := countBytes(objs)
	return c, b, nil
}

func (f *fakeObjectStore) DeletePrefix(_ context.Context, storage, prefix string) (int, error) {
	if err := f.deleteErr[key(storage, prefix)]; err != nil {
		return 0, err
	}
	f.deletes = append(f.deletes, key(storage, prefix))
	return len(f.lists[key(storage, prefix)]), nil
}

// fakeEpisodeStore implements episodeStore with a scripted flip outcome and
// re-read result.
type fakeEpisodeStore struct {
	updateErr error
	updated   []string // ids UpdateStorage was called with

	getEp  *domain.Episode
	getErr error
}

func (f *fakeEpisodeStore) UpdateStorage(_ context.Context, id string, _ string) error {
	f.updated = append(f.updated, id)
	return f.updateErr
}

func (f *fakeEpisodeStore) GetByID(_ context.Context, _ string) (*domain.Episode, error) {
	return f.getEp, f.getErr
}

// objs builds a canned listing of n objects, each `size` bytes.
func objs(n int, size int64) []storageclient.Object {
	out := make([]storageclient.Object, n)
	for i := range out {
		out[i] = storageclient.Object{Key: fmt.Sprintf("o%d", i), Size: size}
	}
	return out
}

func testEpisode() domain.Episode {
	return domain.Episode{
		ID:            "ep-1",
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		MinioPath:     "aeProvider/12345/RAW/3/",
		Storage:       domain.BackendMinio,
		Source:        domain.EpisodeSourceAutocache,
	}
}

func testLog(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.Default()
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestMigrateOne_HappyPath — full sequence: copy, verify, flip, delete local.
func TestMigrateOne_HappyPath(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}
	episodes := &fakeEpisodeStore{}

	ok, bytes := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if !ok || bytes != 300 {
		t.Fatalf("migrateOne = (%v, %d), want (true, 300)", ok, bytes)
	}
	if len(episodes.updated) != 1 {
		t.Fatalf("UpdateStorage calls = %v, want exactly one", episodes.updated)
	}
	if !has(store.deletes, key(domain.BackendMinio, ep.MinioPath)) {
		t.Fatalf("minio prefix not deleted after verified flip; deletes=%v", store.deletes)
	}
	if has(store.deletes, key(domain.BackendS3, ep.MinioPath)) {
		t.Fatalf("s3 prefix must NEVER be deleted on the happy path; deletes=%v", store.deletes)
	}
}

// TestMigrateOne_VerifyMismatchSkips — s3 listing disagrees with minio →
// nothing flipped, nothing deleted on either side.
func TestMigrateOne_VerifyMismatchSkips(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(2, 100), // count mismatch
	}}
	episodes := &fakeEpisodeStore{}

	ok, _ := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if ok {
		t.Fatal("migrateOne succeeded on a verify MISMATCH, want skip")
	}
	if len(episodes.updated) != 0 {
		t.Fatalf("row was flipped despite mismatch; updated=%v", episodes.updated)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("objects deleted despite mismatch; deletes=%v", store.deletes)
	}
}

// TestMigrateOne_VanishedRow — UpdateStorage reports NotFound (evictor won the
// race): the minio prefix is left alone, the orphaned s3 copy is undone, skip.
func TestMigrateOne_VanishedRow(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}
	episodes := &fakeEpisodeStore{updateErr: liberrors.NotFound("episode")}

	ok, _ := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if ok {
		t.Fatal("migrateOne succeeded despite vanished row, want skip")
	}
	if has(store.deletes, key(domain.BackendMinio, ep.MinioPath)) {
		t.Fatalf("minio prefix deleted for a vanished row (evictor owns it); deletes=%v", store.deletes)
	}
	if !has(store.deletes, key(domain.BackendS3, ep.MinioPath)) {
		t.Fatalf("orphaned s3 copy NOT undone for a vanished row; deletes=%v", store.deletes)
	}
}

// TestMigrateOne_FlipFailedRowStillMinio — transient flip failure, re-read
// shows the row still on minio: the s3 copy is undone (it would leak — the
// reconcile pass only revisits rows flipped to s3), minio untouched, skip.
func TestMigrateOne_FlipFailedRowStillMinio(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}
	still := testEpisode() // storage still minio
	episodes := &fakeEpisodeStore{updateErr: errors.New("db connection reset"), getEp: &still}

	ok, _ := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if ok {
		t.Fatal("migrateOne succeeded despite failed flip, want skip")
	}
	if has(store.deletes, key(domain.BackendMinio, ep.MinioPath)) {
		t.Fatalf("minio prefix deleted despite failed flip; deletes=%v", store.deletes)
	}
	if !has(store.deletes, key(domain.BackendS3, ep.MinioPath)) {
		t.Fatalf("leaked s3 copy NOT undone after failed flip; deletes=%v", store.deletes)
	}
}

// TestMigrateOne_FlipFailedConcurrentFlipWon — transient flip failure but the
// re-read shows the row already s3 (a concurrent flip won): success, and
// NOTHING is deleted here (local cleanup belongs to the winning flipper /
// next reconcile).
func TestMigrateOne_FlipFailedConcurrentFlipWon(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}
	flippedRow := testEpisode()
	flippedRow.Storage = domain.BackendS3
	episodes := &fakeEpisodeStore{updateErr: errors.New("serialization failure"), getEp: &flippedRow}

	ok, bytes := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if !ok || bytes != 300 {
		t.Fatalf("migrateOne = (%v, %d), want (true, 300) when concurrent flip won", ok, bytes)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes performed when concurrent flip won; deletes=%v", store.deletes)
	}
}

// TestMigrateOne_FlipFailedRereadFailed — flip fails AND the re-read fails:
// state unknowable, so absolutely nothing is deleted (a possibly-leaked s3
// prefix beats deleting data whose DB state is unknown).
func TestMigrateOne_FlipFailedRereadFailed(t *testing.T) {
	ep := testEpisode()
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}
	episodes := &fakeEpisodeStore{updateErr: errors.New("db down"), getErr: errors.New("db still down")}

	ok, _ := migrateOne(context.Background(), testLog(t), store, episodes, ep)
	if ok {
		t.Fatal("migrateOne succeeded with unknowable row state, want skip")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes performed with unknowable row state; deletes=%v", store.deletes)
	}
}

// TestReconcile_HealsMatchingLeftover — a flipped row with stale minio objects
// whose s3 copy still verifies → minio leftovers deleted, counted as cleaned.
func TestReconcile_HealsMatchingLeftover(t *testing.T) {
	ep := testEpisode()
	ep.Storage = domain.BackendS3
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 100),
	}}

	cleaned, skipped := reconcileLeftovers(context.Background(), testLog(t), store, []domain.Episode{ep})
	if cleaned != 1 || skipped != 0 {
		t.Fatalf("reconcile = (cleaned=%d, skipped=%d), want (1, 0)", cleaned, skipped)
	}
	if !has(store.deletes, key(domain.BackendMinio, ep.MinioPath)) {
		t.Fatalf("matching minio leftovers not deleted; deletes=%v", store.deletes)
	}
	if has(store.deletes, key(domain.BackendS3, ep.MinioPath)) {
		t.Fatalf("reconcile must never delete the s3 side; deletes=%v", store.deletes)
	}
}

// TestReconcile_SkipsMismatchedLeftover — stale minio objects whose s3 copy
// does NOT verify → both sides left untouched, counted as skipped.
func TestReconcile_SkipsMismatchedLeftover(t *testing.T) {
	ep := testEpisode()
	ep.Storage = domain.BackendS3
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendMinio, ep.MinioPath): objs(3, 100),
		key(domain.BackendS3, ep.MinioPath):    objs(3, 90), // byte mismatch
	}}

	cleaned, skipped := reconcileLeftovers(context.Background(), testLog(t), store, []domain.Episode{ep})
	if cleaned != 0 || skipped != 1 {
		t.Fatalf("reconcile = (cleaned=%d, skipped=%d), want (0, 1)", cleaned, skipped)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes performed on mismatched leftover; deletes=%v", store.deletes)
	}
}

// TestReconcile_CleanRowIsNoop — a flipped row whose minio prefix is already
// empty (the steady state) is neither cleaned nor skipped.
func TestReconcile_CleanRowIsNoop(t *testing.T) {
	ep := testEpisode()
	ep.Storage = domain.BackendS3
	store := &fakeObjectStore{lists: map[string][]storageclient.Object{
		key(domain.BackendS3, ep.MinioPath): objs(3, 100),
		// no minio entry → empty listing
	}}

	cleaned, skipped := reconcileLeftovers(context.Background(), testLog(t), store, []domain.Episode{ep})
	if cleaned != 0 || skipped != 0 {
		t.Fatalf("reconcile = (cleaned=%d, skipped=%d), want (0, 0)", cleaned, skipped)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes performed on already-clean row; deletes=%v", store.deletes)
	}
}
