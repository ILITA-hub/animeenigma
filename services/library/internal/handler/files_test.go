package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
)

type fakeStore struct {
	objs    []storageclient.Object
	dlURL   string
	deleted string
}

func (f *fakeStore) List(context.Context, string, string) ([]storageclient.Object, error) {
	return f.objs, nil
}
func (f *fakeStore) DownloadURL(context.Context, string, string) (string, error) { return f.dlURL, nil }
func (f *fakeStore) DeletePrefix(_ context.Context, _, p string) error           { f.deleted = p; return nil }

type fakeEpisodes struct{ pool []domain.Episode }

func (f *fakeEpisodes) ListPool(context.Context) ([]domain.Episode, error) { return f.pool, nil }

type fakeConfig struct{}

func (fakeConfig) Get(context.Context) (*domain.AutocacheConfig, error) {
	return &domain.AutocacheConfig{AdminFreshDays: 3650}, nil
}

type fakeEvictor struct{ deletedID string }

func (f *fakeEvictor) DeleteEpisodeByID(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

type fakeActive struct{ set map[string]struct{} }

func (f *fakeActive) Infohashes(context.Context) (map[string]struct{}, error) { return f.set, nil }

func newTestFiles(t *testing.T, store filesObjectStore, eps filesEpisodeIndex, ev filesEpisodeEvictor, act filesActive) *FilesHandler {
	t.Helper()
	return NewFilesHandler(service.NewWorkDir(t.TempDir()), store, eps, fakeConfig{}, ev, act, nil)
}

func TestBrowse_ObjectFolderSynthesisAndEpisodeAnnotation(t *testing.T) {
	store := &fakeStore{objs: []storageclient.Object{
		{Key: "frieren/s1/e01/e01.m3u8", Size: 2000},
		{Key: "frieren/s1/e01/e01_1080p.ts", Size: 240000000},
		{Key: "frieren/s1/e02/e02.m3u8", Size: 2000},
	}}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{
		{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", ShikimoriID: "1", EpisodeNumber: n, Source: domain.EpisodeSourceAdmin},
	}}
	h := newTestFiles(t, store, eps, &fakeEvictor{}, &fakeActive{})

	req := httptest.NewRequest(http.MethodGet, "/api/library/files?domain=minio&prefix=frieren/s1/", nil)
	rw := httptest.NewRecorder()
	h.Browse(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status %d", rw.Code)
	}

	// Expect two dir entries e01, e02; e01 annotated with episode ep-1.
	var parsed struct {
		Data browseResponseDTO `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rw.Body.String())
	}
	entries := parsed.Data.Entries
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2; body=%s", len(entries), rw.Body.String())
	}

	e01 := entries[0]
	if e01.Kind != "dir" || e01.Name != "e01" {
		t.Fatalf("entries[0] = %+v, want dir named e01", e01)
	}
	if e01.Size != 240002000 {
		t.Errorf("entries[0].Size = %d, want 240002000 (sum of e01's two objects)", e01.Size)
	}
	if e01.Episode == nil {
		t.Fatalf("entries[0].Episode = nil, want annotated with ep-1")
	}
	if e01.Episode.EpisodeID != "ep-1" {
		t.Errorf("entries[0].Episode.EpisodeID = %q, want ep-1", e01.Episode.EpisodeID)
	}
	if e01.Episode.ShikimoriID != "1" {
		t.Errorf("entries[0].Episode.ShikimoriID = %q, want 1", e01.Episode.ShikimoriID)
	}
	if e01.Episode.Episode == nil || *e01.Episode.Episode != 1 {
		t.Errorf("entries[0].Episode.Episode = %v, want pointer to 1", e01.Episode.Episode)
	}
	if e01.Episode.Source != string(domain.EpisodeSourceAdmin) {
		t.Errorf("entries[0].Episode.Source = %q, want admin", e01.Episode.Source)
	}
	if e01.Episode.Freshness != "stale" {
		t.Errorf("entries[0].Episode.Freshness = %q, want stale (no DownloadedAt/LastFetchAt set)", e01.Episode.Freshness)
	}

	e02 := entries[1]
	if e02.Kind != "dir" || e02.Name != "e02" {
		t.Fatalf("entries[1] = %+v, want dir named e02", e02)
	}
	if e02.Episode != nil {
		t.Errorf("entries[1].Episode = %+v, want nil (no matching pool row)", e02.Episode)
	}
}

type errConfig struct{}

func (errConfig) Get(context.Context) (*domain.AutocacheConfig, error) {
	return nil, errors.New("config unavailable")
}

// TestBrowse_ConfigFetchErrorDoesNotPanic guards the nil-*AutocacheConfig
// panic in autocache.Classify: if filesConfig.Get fails, Browse must fall back
// to a zero-value config instead of passing nil through, and still return 200
// with the annotated (maximally-stale) entry.
func TestBrowse_ConfigFetchErrorDoesNotPanic(t *testing.T) {
	store := &fakeStore{objs: []storageclient.Object{
		{Key: "frieren/s1/e01/e01.m3u8", Size: 2000},
	}}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{
		{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", ShikimoriID: "1", EpisodeNumber: n, Source: domain.EpisodeSourceAdmin},
	}}
	h := NewFilesHandler(service.NewWorkDir(t.TempDir()), store, eps, errConfig{}, &fakeEvictor{}, &fakeActive{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/files?domain=minio&prefix=frieren/s1/", nil)
	rw := httptest.NewRecorder()
	h.Browse(rw, req) // must not panic even though config.Get errors

	if rw.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
	var parsed struct {
		Data browseResponseDTO `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rw.Body.String())
	}
	if len(parsed.Data.Entries) != 1 || parsed.Data.Entries[0].Episode == nil {
		t.Fatalf("entries = %+v, want one annotated dir", parsed.Data.Entries)
	}
	if parsed.Data.Entries[0].Episode.Freshness != "stale" {
		t.Errorf("Freshness = %q, want stale (fallback zero-value config)", parsed.Data.Entries[0].Episode.Freshness)
	}
}

func TestBrowse_BadDomain400(t *testing.T) {
	h := newTestFiles(t, &fakeStore{}, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})
	req := httptest.NewRequest(http.MethodGet, "/api/library/files?domain=zzz", nil)
	rw := httptest.NewRecorder()
	h.Browse(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rw.Code)
	}
}

// TestDownload_WorkDirStreamsFile confirms domain=work reads straight off disk
// (through the WorkDir jail) and sets a Content-Disposition attachment header
// naming the file.
func TestDownload_WorkDirStreamsFile(t *testing.T) {
	root := t.TempDir()
	content := []byte("hello from the torrent working dir")
	if err := os.WriteFile(filepath.Join(root, "movie.mkv"), content, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	h := NewFilesHandler(service.NewWorkDir(root), &fakeStore{}, &fakeEpisodes{}, fakeConfig{}, &fakeEvictor{}, &fakeActive{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/files/download?domain=work&key=movie.mkv", nil)
	rw := httptest.NewRecorder()
	h.Download(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
	if got := rw.Body.String(); got != string(content) {
		t.Errorf("body = %q, want %q", got, string(content))
	}
	if cd := rw.Header().Get("Content-Disposition"); cd != `attachment; filename="movie.mkv"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

// TestDownload_ObjectUsesPresignedFetch confirms domain=minio|s3 resolves a
// presigned URL via the store seam and streams the bytes fetched from it
// (through the real httpGet seam, pointed at a local httptest.Server) back to
// the caller, with Content-Disposition/Content-Type carried through.
func TestDownload_ObjectUsesPresignedFetch(t *testing.T) {
	payload := []byte("segment bytes served from the presigned url")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer ts.Close()

	store := &fakeStore{dlURL: ts.URL}
	h := newTestFiles(t, store, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})

	req := httptest.NewRequest(http.MethodGet, "/api/library/files/download?domain=minio&key=frieren/s1/e01/e01_1080p.ts", nil)
	rw := httptest.NewRecorder()
	h.Download(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
	if got := rw.Body.String(); got != string(payload) {
		t.Errorf("body = %q, want %q", got, string(payload))
	}
	if cd := rw.Header().Get("Content-Disposition"); cd != `attachment; filename="e01_1080p.ts"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if ct := rw.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("Content-Type = %q, want video/mp2t", ct)
	}
}

// TestDelete_EpisodeRoutesToEvictor confirms that deleting an object key which
// maps to a library_episodes row (via MinioPath) goes through the reconciled
// evictor.DeleteEpisodeByID path, never a raw store.DeletePrefix.
func TestDelete_EpisodeRoutesToEvictor(t *testing.T) {
	store := &fakeStore{}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", EpisodeNumber: n}}}
	ev := &fakeEvictor{}
	h := newTestFiles(t, store, eps, ev, &fakeActive{})

	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=frieren/s1/e01/", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status %d", rw.Code)
	}
	if ev.deletedID != "ep-1" {
		t.Fatalf("expected evictor delete ep-1, got %q", ev.deletedID)
	}
	if store.deleted != "" {
		t.Fatal("must not raw-delete an episode prefix")
	}
}

// TestDelete_OrphanNeedsConfirm confirms an object key with no matching
// episode row is refused with 409 unless ?confirm=1 is present, in which case
// it falls through to a raw store.DeletePrefix.
func TestDelete_OrphanNeedsConfirm(t *testing.T) {
	store := &fakeStore{}
	h := newTestFiles(t, store, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})
	// no confirm → 409
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=stray/file.bin", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusConflict {
		t.Fatalf("status %d", rw.Code)
	}
	// with confirm → deletes prefix
	req2 := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=stray/file.bin&confirm=1", nil)
	rw2 := httptest.NewRecorder()
	h.Delete(rw2, req2)
	if rw2.Code != http.StatusOK || store.deleted != "stray/file.bin" {
		t.Fatalf("status %d deleted %q", rw2.Code, store.deleted)
	}
}

// TestDelete_WorkDirActiveTorrentRefused confirms a domain=work delete is
// refused with 409 when the top-level infohash segment of the key is still an
// in-flight job (per ActiveTorrents.Infohashes).
func TestDelete_WorkDirActiveTorrentRefused(t *testing.T) {
	const ih = "abcd"
	h := newTestFiles(t, &fakeStore{}, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{set: map[string]struct{}{ih: {}}})
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=work&key=abcd", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusConflict {
		t.Fatalf("expected 409 for active torrent, got %d", rw.Code)
	}
	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rw.Body.String())
	}
	if parsed.Data["reason"] != "torrent_active" {
		t.Errorf("reason = %q, want torrent_active", parsed.Data["reason"])
	}
}

// TestDelete_ObjectUnderEpisodePrefixRefused confirms that deleting an
// individual object key that lives UNDER an episode's MinioPath (but isn't
// the prefix itself — e.g. a .ts segment inside the episode folder) is
// refused with 409 rather than falling through to a raw orphan delete, which
// would silently break the episode's HLS with no DB reconcile.
func TestDelete_ObjectUnderEpisodePrefixRefused(t *testing.T) {
	store := &fakeStore{}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", EpisodeNumber: n}}}
	ev := &fakeEvictor{}
	h := newTestFiles(t, store, eps, ev, &fakeActive{})

	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=frieren/s1/e01/e01_1080p.ts", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)

	if rw.Code != http.StatusConflict {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rw.Body.String())
	}
	if parsed.Data["reason"] != "episode_member" {
		t.Errorf("reason = %q, want episode_member", parsed.Data["reason"])
	}
	if ev.deletedID != "" {
		t.Errorf("must not call evictor.DeleteEpisodeByID, got %q", ev.deletedID)
	}
	if store.deleted != "" {
		t.Errorf("must not raw-delete an object under an episode prefix, got %q", store.deleted)
	}
}

// TestDelete_ObjectContainingEpisodesRefused confirms that deleting a folder
// that CONTAINS one or more episodes (e.g. a season folder above the episode
// dirs) is refused with 409 rather than falling through to a raw DeletePrefix,
// which would wipe every episode beneath it and orphan all their
// library_episodes rows. This is the ancestor direction of the overlap check.
func TestDelete_ObjectContainingEpisodesRefused(t *testing.T) {
	store := &fakeStore{}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", EpisodeNumber: n}}}
	ev := &fakeEvictor{}
	h := newTestFiles(t, store, eps, ev, &fakeActive{})

	// confirm=1 present: even an explicit raw-delete confirm must not wipe a
	// folder that contains episodes.
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=frieren/s1/&confirm=1", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)

	if rw.Code != http.StatusConflict {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rw.Body.String())
	}
	if parsed.Data["reason"] != "episode_member" {
		t.Errorf("reason = %q, want episode_member", parsed.Data["reason"])
	}
	if store.deleted != "" {
		t.Errorf("must not raw-delete a folder containing episodes, got %q", store.deleted)
	}
	if ev.deletedID != "" {
		t.Errorf("must not route a containing folder through the evictor, got %q", ev.deletedID)
	}
}

// TestDelete_RootKeyRejected confirms key="/" (which would otherwise trim to
// "" and reach store.DeletePrefix(dom, "/") — a bucket-wide delete) is
// rejected with 400 alongside the existing key=="" guard.
func TestDelete_RootKeyRejected(t *testing.T) {
	h := newTestFiles(t, &fakeStore{}, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=%2F", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body=%s", rw.Code, rw.Body.String())
	}
}
