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
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[{"episode_number":1,"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","track":"dub","audio_lang":"eng","quality":"1080p"},{"episode_number":2,"minio_url":"http://minio:9000/raw-library/57466/2/playlist.m3u8"}]}}`)
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
	if got.Episodes[0].Track != "dub" || got.Episodes[0].AudioLang != "eng" || got.Episodes[0].Quality != "1080p" {
		t.Errorf("episode[0] audio facts = %+v, want track=dub audio_lang=eng quality=1080p", got.Episodes[0])
	}
	if got.Episodes[1].Track != "" || got.Episodes[1].AudioLang != "" || got.Episodes[1].Quality != "" {
		t.Errorf("episode[1] audio facts = %+v, want all empty (omitted by fake response)", got.Episodes[1])
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

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
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

// TestRawResolver_GetLibraryStream_StoryboardSigned proves that when the
// library's episode response carries a storyboard_url, GetLibraryStream
// mints a signed RawStoryboard alongside the playlist — same trust path
// (streamsign.Sign) as the master-playlist URL.
func TestRawResolver_GetLibraryStream_StoryboardSigned(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	const storyboardURL = "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard.vtt"
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100,"storyboard_url":%q}}`, storyboardURL)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.Storyboard == nil {
		t.Fatal("expected non-nil Storyboard when library returns storyboard_url")
	}
	if got.Storyboard.URL != storyboardURL {
		t.Errorf("Storyboard.URL = %q, want %q", got.Storyboard.URL, storyboardURL)
	}
	if got.Storyboard.Exp == "" || got.Storyboard.Sig == "" {
		t.Errorf("expected signed storyboard URL (exp/sig), got exp=%q sig=%q", got.Storyboard.Exp, got.Storyboard.Sig)
	}
}

// TestRawResolver_GetLibraryStream_NoStoryboardWhenAbsent proves that when
// the library's episode response has no storyboard_url (episode encoded
// before the storyboard pass shipped, or the ffmpeg storyboard pass failed
// best-effort), RawStream.Storyboard stays nil rather than a zero-value
// struct — so the frontend's `if (stream.storyboard)` check works.
func TestRawResolver_GetLibraryStream_NoStoryboardWhenAbsent(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.Storyboard != nil {
		t.Errorf("expected nil Storyboard when library omits storyboard_url, got %+v", got.Storyboard)
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

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 99, "", "")
	if err == nil {
		t.Fatal("expected NotFound error when episode absent from library")
	}
}

// ---- Task 5: dual-storage union + ?server= selection ----

// dualStorageLibHandler fakes GET /api/library/episodes/{sk}/{ep} so a
// ?storage=minio request hits minioURL and ?storage=s3 hits s3URL. An empty
// (unset) storage query param — as the library service itself does — is
// treated as "prefer minio". Empty *URL means that storage 404s (episode
// absent there).
func dualStorageLibHandler(minioURL, s3URL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		storage := r.URL.Query().Get("storage")
		var url string
		switch storage {
		case "minio":
			url = minioURL
		case "s3":
			url = s3URL
		default:
			if minioURL != "" {
				url = minioURL
			} else {
				url = s3URL
			}
		}
		if url == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"data":{"minio_url":%q,"storage":%q}}`, url, storage)
	}
}

// dualStorageLibServer wraps dualStorageLibHandler in an httptest.Server.
func dualStorageLibServer(t *testing.T, minioURL, s3URL string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(dualStorageLibHandler(minioURL, s3URL))
}

// TestRawResolver_GetLibraryStream_DualStorage_ServersPresentDefaultMinio
// proves that when an episode exists on BOTH storages, the resolved stream
// carries a Servers list (Local/Cloud) AND the default (server="") pick is
// the local minio copy.
func TestRawResolver_GetLibraryStream_DualStorage_ServersPresentDefaultMinio(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	const (
		minioURL = "http://minio:9000/raw-library/57466/1/playlist.m3u8"
		s3URL    = "https://s3.firstvds.ru/raw-library/57466/1/playlist.m3u8"
	)
	libSrv := dualStorageLibServer(t, minioURL, s3URL)
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.URL != minioURL {
		t.Errorf("URL = %q, want the minio copy %q (default prefers local)", got.URL, minioURL)
	}
	if len(got.Servers) != 2 {
		t.Fatalf("Servers = %+v, want 2 entries (dual-storage)", got.Servers)
	}
	want := []RawServer{{ID: "minio", Label: "Local"}, {ID: "s3", Label: "Cloud"}}
	if got.Servers[0] != want[0] || got.Servers[1] != want[1] {
		t.Errorf("Servers = %+v, want %+v", got.Servers, want)
	}
}

// TestRawResolver_GetLibraryStream_S3Only_NoServersResolvesS3 proves that an
// episode present ONLY on s3 has no Servers list (nothing to choose between)
// and the default (server="") resolution still finds it via the s3 fallback.
func TestRawResolver_GetLibraryStream_S3Only_NoServersResolvesS3(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	const s3URL = "https://s3.firstvds.ru/raw-library/57466/1/playlist.m3u8"
	libSrv := dualStorageLibServer(t, "", s3URL)
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.URL != s3URL {
		t.Errorf("URL = %q, want the s3 copy %q", got.URL, s3URL)
	}
	if got.Servers != nil {
		t.Errorf("Servers = %+v, want nil (single-copy s3-only episode)", got.Servers)
	}
}

// TestRawResolver_GetLibraryStream_ExplicitServerS3_SignsS3URL proves an
// explicit ?server=s3 request on a dual-storage episode resolves + signs the
// s3 copy (not the minio default), while still reporting Servers (the
// episode is genuinely dual-present).
func TestRawResolver_GetLibraryStream_ExplicitServerS3_SignsS3URL(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	const (
		minioURL = "http://minio:9000/raw-library/57466/1/playlist.m3u8"
		s3URL    = "https://s3.firstvds.ru/raw-library/57466/1/playlist.m3u8"
	)
	libSrv := dualStorageLibServer(t, minioURL, s3URL)
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "s3")
	if err != nil {
		t.Fatalf("GetLibraryStream: %v", err)
	}
	if got.URL != s3URL {
		t.Errorf("URL = %q, want the explicitly-requested s3 copy %q", got.URL, s3URL)
	}
	if got.Exp == "" || got.Sig == "" {
		t.Errorf("expected the s3 URL to be signed same as minio, got exp=%q sig=%q", got.Exp, got.Sig)
	}
	if len(got.Servers) != 2 {
		t.Errorf("Servers = %+v, want 2 entries (dual-storage, regardless of which was requested)", got.Servers)
	}
}

// TestRawResolver_GetLibraryStream_ExplicitServerNotOnThatStorage_404NotMISS
// proves that requesting a storage the episode doesn't have (while the other
// storage DOES have it) returns a clean NotFound — NOT a 500, and NOT a
// genuine backfill-demand MISS signal (the episode isn't actually missing
// from the library, just from the requested copy).
func TestRawResolver_GetLibraryStream_ExplicitServerNotOnThatStorage_404NotMISS(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	const minioURL = "http://minio:9000/raw-library/57466/1/playlist.m3u8"
	internalCalls := make(chan string, 4)
	episodeHandler := dualStorageLibHandler(minioURL, "") // minio-only
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/library/autocache/") {
			internalCalls <- r.URL.Path
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		episodeHandler(w, r)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "s3")
	if err == nil {
		t.Fatal("expected NotFound when the episode isn't on the explicitly-requested storage")
	}
	if !isNotFound(err) {
		t.Fatalf("expected a clean NotFound (400/404), not a 500-shaped error: %v", err)
	}
	select {
	case p := <-internalCalls:
		t.Errorf("must NOT fire a backfill demand for a wrong-storage request (episode exists on minio), got call to %q", p)
	case <-time.After(100 * time.Millisecond):
		// expected: no signal fired
	}
}

// TestRawResolver_GetLibraryStream_InvalidServer_400 proves an unrecognized
// ?server= value is rejected with errors.InvalidInput (→ 400 via
// httputil.Error), mirroring how handler/scraper.go validates its own
// server param.
func TestRawResolver_GetLibraryStream_InvalidServer_400(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))
	r := NewRawResolver(nil, animeRepo, cacheC, nil)

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "gcs")
	if err == nil {
		t.Fatal("expected an error for an invalid server value")
	}
	appErr, ok := errors.IsAppError(err)
	if !ok || appErr.Code != errors.CodeInvalidInput {
		t.Fatalf("got err %v, want a libs/errors AppError with CodeInvalidInput", err)
	}
}

// ---- Task 5: GetLibraryEpisodes union-dedupe by episode_number ----

// TestRawResolver_GetLibraryEpisodes_DualStorageDedupedByNumber proves that
// when ListEpisodes returns two rows for the same episode_number (one per
// storage — the union the library API now returns), GetLibraryEpisodes
// collapses them to a single entry so ae aggregates (capabilities,
// AeTitleInfo, partial_library) never double-count a dual-present episode.
func TestRawResolver_GetLibraryEpisodes_DualStorageDedupedByNumber(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"episodes":[
			{"episode_number":1,"minio_url":"https://s3.firstvds.ru/x/1/playlist.m3u8","storage":"s3","track":"raw","quality":"720p"},
			{"episode_number":1,"minio_url":"http://minio:9000/x/1/playlist.m3u8","storage":"minio","track":"dub","audio_lang":"eng","quality":"1080p"},
			{"episode_number":2,"minio_url":"http://minio:9000/x/2/playlist.m3u8","storage":"minio"}
		]}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(libClient, animeRepo, cacheC, nil)

	got, err := r.GetLibraryEpisodes(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("GetLibraryEpisodes: %v", err)
	}
	if len(got.Episodes) != 2 {
		t.Fatalf("len(Episodes) = %d, want 2 (deduped by episode_number), got %+v", len(got.Episodes), got.Episodes)
	}
	// The minio row must win when both storages hold the same episode number.
	if got.Episodes[0].Track != "dub" || got.Episodes[0].AudioLang != "eng" || got.Episodes[0].Quality != "1080p" {
		t.Errorf("episode[0] (deduped) = %+v, want the minio row's audio facts (dub/eng/1080p)", got.Episodes[0])
	}
}

// TestRawResolver_AeTitleInfo_DualStorageDoesNotDoubleCount proves
// AeTitleInfo (which every ae capability aggregate reads) sees the deduped
// count — a dual-present single dub episode still yields exactly one
// CoversFirstEpisode+dub verdict, not an inflated one.
func TestRawResolver_AeTitleInfo_DualStorageDoesNotDoubleCount(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[
		{"episode_number":1,"minio_url":"http://minio:9000/x/1/playlist.m3u8","storage":"minio","track":"dub","audio_lang":"eng","quality":"1080p"},
		{"episode_number":1,"minio_url":"https://s3.firstvds.ru/x/1/playlist.m3u8","storage":"s3","track":"dub","audio_lang":"eng","quality":"1080p"}
	]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if !got.Present || !got.CoversFirstEpisode {
		t.Fatalf("got %+v, want Present+CoversFirstEpisode true", got)
	}
	if got.Track != "dub" || got.AudioLang != "eng" {
		t.Errorf("got Track=%q AudioLang=%q, want dub/eng", got.Track, got.AudioLang)
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

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
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

	_, err := r.GetLibraryStream(context.Background(), testAnimeID, 99, "", "")
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

	got, err := r.GetLibraryStream(context.Background(), testAnimeID, 1, "", "")
	if err != nil {
		t.Fatalf("a failing serve-signal must not fail the resolution, got err %v", err)
	}
	if got == nil || got.Source != "library" || got.Type != "hls" {
		t.Fatalf("resolution result changed under signal failure: %+v", got)
	}
	// Give the goroutine a moment to run+fail without affecting anything.
	time.Sleep(50 * time.Millisecond)
}

// ---- Phase C: AeTitleInfo per-title audio aggregation ----

// newAeInfoResolver spins up a fake library server returning the given raw
// episodes JSON body and returns a RawResolver wired to it.
func newAeInfoResolver(t *testing.T, episodesJSON string) *RawResolver {
	t.Helper()
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, episodesJSON)
	}))
	t.Cleanup(libSrv.Close)

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	return NewRawResolver(libClient, animeRepo, cacheC, nil)
}

// TestRawResolver_AeTitleInfo_AnyDubEpisodeWins proves that a single dub
// episode among otherwise-raw episodes makes the whole title a dub, and
// records that episode's audio_lang + the first non-empty quality seen.
func TestRawResolver_AeTitleInfo_AnyDubEpisodeWins(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[
		{"episode_number":1,"minio_url":"http://minio:9000/x/1/playlist.m3u8","track":"raw","quality":"720p"},
		{"episode_number":2,"minio_url":"http://minio:9000/x/2/playlist.m3u8","track":"dub","audio_lang":"rus","quality":"1080p"}
	]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if !got.Present {
		t.Fatal("Present = false, want true (episodes exist)")
	}
	if got.Track != "dub" || got.AudioLang != "rus" {
		t.Errorf("got Track=%q AudioLang=%q, want dub/rus", got.Track, got.AudioLang)
	}
	if got.Quality != "720p" {
		t.Errorf("Quality = %q, want %q (first non-empty episode quality)", got.Quality, "720p")
	}
}

// TestRawResolver_AeTitleInfo_NoDubIsRaw proves that with no dub episodes
// present, the title aggregates to Track="raw" (original/sub) and no
// AudioLang.
func TestRawResolver_AeTitleInfo_NoDubIsRaw(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[
		{"episode_number":1,"minio_url":"http://minio:9000/x/1/playlist.m3u8","quality":"1080p"},
		{"episode_number":2,"minio_url":"http://minio:9000/x/2/playlist.m3u8"}
	]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if !got.Present {
		t.Fatal("Present = false, want true (episodes exist)")
	}
	if got.Track != "raw" {
		t.Errorf("Track = %q, want %q when no episode is a dub", got.Track, "raw")
	}
	if got.AudioLang != "" {
		t.Errorf("AudioLang = %q, want empty when no episode is a dub", got.AudioLang)
	}
	if got.Quality != "1080p" {
		t.Errorf("Quality = %q, want %q", got.Quality, "1080p")
	}
}

// A library that holds episode 1 sets CoversFirstEpisode — a complete/early
// library that can serve a fresh open on ep 1.
func TestRawResolver_AeTitleInfo_CoversFirstEpisode(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[
		{"episode_number":1,"minio_url":"http://minio:9000/x/1/playlist.m3u8","quality":"1080p"},
		{"episode_number":2,"minio_url":"http://minio:9000/x/2/playlist.m3u8"}
	]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if !got.CoversFirstEpisode {
		t.Errorf("CoversFirstEpisode = false, want true (library holds ep 1)")
	}
}

// A late-only auto-cache (e.g. Frieren ep 27 of 28) does NOT cover ep 1 — the
// FE keeps it out of the fresh-open smart default so the player opens ep 1 from
// a full source.
func TestRawResolver_AeTitleInfo_LateOnlyDoesNotCoverFirst(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[
		{"episode_number":27,"minio_url":"http://minio:9000/x/27/playlist.m3u8","track":"raw","quality":"1080p"}
	]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if !got.Present {
		t.Fatal("Present = false, want true (a late episode exists)")
	}
	if got.CoversFirstEpisode {
		t.Errorf("CoversFirstEpisode = true, want false (library holds only ep 27)")
	}
}

// TestRawResolver_AeTitleInfo_EmptyLibraryNotPresent proves an
// empty/unavailable library yields a zero AeInfo (Present=false), matching
// GetLibraryEpisodes' own Available=false contract.
func TestRawResolver_AeTitleInfo_EmptyLibraryNotPresent(t *testing.T) {
	r := newAeInfoResolver(t, `{"success":true,"data":{"episodes":[]}}`)

	got, err := r.AeTitleInfo(context.Background(), testAnimeID)
	if err != nil {
		t.Fatalf("AeTitleInfo: %v", err)
	}
	if got != (AeInfo{}) {
		t.Errorf("got %+v, want zero AeInfo for an empty library", got)
	}
}
