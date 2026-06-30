package service

// Tests for the library-only raw (JP-audio) resolver. The AllAnime
// backend was removed (2026-06-22): its sources decode to a
// Cloudflare-Turnstile-walled /apivtwo/clock endpoint, unsolvable from
// our egress, and its "Ok" (ok.ru) sources are now served by the
// dedicated 'okru' scraper provider. RAW (JP audio, no burned-in subs,
// soft Jimaku overlay) is therefore served from the self-hosted library
// (MinIO HLS ladder) only — there is no fallback path.
//
// These tests verify:
//   - library 200 → MinIO stream (Source="library", signed).
//   - library 404 → NotFound (no fallback).
//   - GetEpisodes empty / no-ShikimoriID → {Episodes:[], Available:false}.
//   - has_raw lazy backfill on a library hit.
//   - the existing GetLibraryEpisodes / GetLibraryStream surface (ae +
//     serve-signal) is unchanged.
//
// Tests require a reachable Redis (defaults to localhost:6379, DB 14 for
// isolation). On a missing Redis we t.Skipf.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---- Test infrastructure ----

func newTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 14})
	if err != nil {
		t.Skipf("redis unreachable at %s:%d (%v); skipping resolver test", host, port, err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() {
		_ = c.Client().FlushDB(context.Background()).Err()
		_ = c.Close()
	})
	return c
}

func newTestDBWithAnime(t *testing.T, anime *domain.Anime) (*gorm.DB, *repo.AnimeRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Minimal animes table — only the columns the resolver reads/writes.
	stmts := []string{
		`CREATE TABLE genres (
			id TEXT PRIMARY KEY,
			name TEXT
		)`,
		`CREATE TABLE anime_genres (
			anime_id TEXT,
			genre_id TEXT
		)`,
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_en TEXT,
			name_ru TEXT,
			name_jp TEXT,
			description TEXT,
			year INTEGER DEFAULT 0,
			season TEXT,
			status TEXT DEFAULT 'released',
			kind TEXT,
			rating TEXT,
			material_source TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0,
			episode_duration INTEGER DEFAULT 0,
			score REAL DEFAULT 0,
			poster_url TEXT,
			shikimori_id TEXT,
			mal_id TEXT,
			anilist_id TEXT,
			imdb_id TEXT,
			tmdb_id TEXT,
			has_video INTEGER DEFAULT 0,
			has_dub INTEGER DEFAULT 0,
			has_kodik INTEGER DEFAULT 0,
			has_animelib INTEGER DEFAULT 0,
			has_raw INTEGER DEFAULT 0,
			hidden INTEGER DEFAULT 0,
			sort_priority INTEGER DEFAULT 0,
			next_episode_at DATETIME,
			aired_on DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("exec ddl: %v", err)
		}
	}
	if err := db.Exec(`INSERT INTO animes (id, name, name_en, name_jp, shikimori_id, has_raw, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		anime.ID, anime.Name, anime.NameEN, anime.NameJP, anime.ShikimoriID,
		boolToInt(anime.HasRaw),
		time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("insert anime: %v", err)
	}
	return db, repo.NewAnimeRepository(db)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isNotFound reports whether err is a libs/errors AppError with CodeNotFound.
func isNotFound(err error) bool {
	if appErr, ok := errors.IsAppError(err); ok {
		return appErr.Code == errors.CodeNotFound
	}
	return false
}

// ---- Tests ----

const (
	testAnimeID     = "test-anime-uuid-0001"
	testShikimoriID = "57466"
)

func makeAnime(hasRaw bool, shikimoriID string) *domain.Anime {
	return &domain.Anime{
		ID:          testAnimeID,
		Name:        "Bocchi the Rock",
		NameEN:      "Bocchi the Rock!",
		NameJP:      "ぼっち・ざ・ろっく！",
		ShikimoriID: shikimoriID,
		HasRaw:      hasRaw,
	}
}

// ---- GetLibraryEpisodes / GetLibraryStream (ae provider surface) — unchanged ----

func TestRawResolver_GetLibraryEpisodes_HappyPath(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/library/episodes/"+testShikimoriID {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[{"episode_number":1,"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8"},{"episode_number":2,"minio_url":"http://minio:9000/raw-library/57466/2/playlist.m3u8"}]}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryEpisodes(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("GetLibraryEpisodes: %v", err)
	}
	if !got.Available || got.Source != "library" {
		t.Errorf("got Available=%v Source=%q, want true/library", got.Available, got.Source)
	}
	if len(got.Episodes) != 2 || got.Episodes[0].Number != 1 || got.Episodes[1].Number != 2 {
		t.Errorf("episodes = %+v", got.Episodes)
	}
}

func TestRawResolver_GetLibraryEpisodes_EmptyNotAvailable(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[]}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryEpisodes(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("GetLibraryEpisodes: %v", err)
	}
	if got.Available {
		t.Errorf("Available = true, want false for empty library")
	}
	if got.Episodes == nil {
		t.Errorf("Episodes must be a non-nil empty slice (JSON [])")
	}
}

func TestRawResolver_GetLibraryStream_SignedAndLibraryOnly(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.Source != "library" || got.Type != "hls" {
		t.Errorf("got Source=%q Type=%q", got.Source, got.Type)
	}
	if got.Exp == "" || got.Sig == "" {
		t.Errorf("expected signed minio URL (exp/sig), got exp=%q sig=%q", got.Exp, got.Sig)
	}
}

func TestRawResolver_GetLibraryStream_404WhenAbsent(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 99, "")
	if err == nil {
		t.Fatal("expected NotFound error when episode absent from library")
	}
}

// ---- Phase 08-03: best-effort serve-signal fire on HIT / MISS ----

// waitForPath blocks up to a deadline for a path to arrive on the signal
// channel, asserting the expected internal endpoint was hit by the
// fire-and-forget goroutine (which races the synchronous return).
func waitForPath(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Errorf("internal call path = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for internal call to %q", want)
	}
}

func TestRawResolver_GetLibraryStream_HIT_FiresRecordFetch(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	internalCalls := make(chan string, 4)
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/library/autocache/") {
			internalCalls <- r.URL.Path
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		// GetEpisode HIT
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetLibraryStream HIT: %v", err)
	}
	if got == nil || got.Source != "library" {
		t.Fatalf("HIT must still return the library stream unchanged, got %+v", got)
	}
	waitForPath(t, internalCalls, "/internal/library/autocache/fetch")
}

func TestRawResolver_GetLibraryStream_MISS_FiresRecordDemand(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	internalCalls := make(chan string, 4)
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/library/autocache/") {
			internalCalls <- r.URL.Path
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		// GetEpisode MISS
		w.WriteHeader(http.StatusNotFound)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 99, "")
	if err == nil {
		t.Fatal("MISS must still return NotFound unchanged")
	}
	waitForPath(t, internalCalls, "/internal/library/autocache/demand")
}

// TestRawResolver_GetLibraryStream_SignalFailureDoesNotAffectResult proves
// the resolution result is byte-for-byte unchanged even when the internal
// serve-signal call fails (500) — best-effort, drop-on-failure.
func TestRawResolver_GetLibraryStream_SignalFailureDoesNotAffectResult(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/library/autocache/") {
			w.WriteHeader(http.StatusInternalServerError) // signal call fails
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("a failing serve-signal must not fail the resolution, got err %v", err)
	}
	if got == nil || got.Source != "library" || got.Type != "hls" {
		t.Fatalf("resolution result changed under signal failure: %+v", got)
	}
	// Give the goroutine a moment to run+fail without affecting anything.
	time.Sleep(50 * time.Millisecond)
}
