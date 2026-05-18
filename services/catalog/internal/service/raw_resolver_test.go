package service

// Phase 06 (workstream raw-jp / v0.2) tests for the hybrid raw
// resolver. Verifies the library-first branch, source-decision cache
// behavior, RawStream.Source population, backward-compat normalization
// of older raw:stream:* entries, and the fall-through to AllAnime on
// library 404 / 5xx / timeout / nil-client / empty-shikimori_id.
//
// Tests require a reachable Redis (defaults to localhost:6379, DB 14
// for isolation from the SetNX tests which use DB 15). On a missing
// Redis we t.Skipf — the live smoke in Task 6 also exercises the
// behavior end-to-end.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/allanime"
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

// rewriteTransport sends every HTTP request to the given host. Used
// to point the AllAnime client at our httptest mock.
type rewriteTransport struct {
	to   string
	base http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = rt.to
	return rt.base.RoundTrip(req)
}

// newAllAnimeMockClient builds an AllAnime client whose configured
// domain points at the given httptest server. We expose only the
// public NewClient (private fields are mutated via the package's own
// test seam). For Phase 06 we never exercise the AllAnime path in
// the library-hit cases — but several cases DO need it to succeed
// after the library lookup falls through.
func newAllAnimeMockClient(t *testing.T, srv *httptest.Server) *allanime.Client {
	t.Helper()
	c := allanime.NewClient(allanime.Config{
		Domains:          []string{"mock.test"},
		QuerySearchSHA:   "sha-search",
		QueryEpisodesSHA: "sha-eps",
		QuerySourcesSHA:  "sha-src",
		HTTPTimeout:      2 * time.Second,
		Referer:          "https://test/",
		UserAgent:        "test-agent",
	})
	u, _ := url.Parse(srv.URL)
	allanime.SetHTTPClientForTest(c, &http.Client{
		Timeout: 2 * time.Second,
		Transport: &rewriteTransport{to: u.Host, base: http.DefaultTransport},
	})
	return c
}

// allAnimeOKHandler returns a chained handler that responds to the
// three persisted-query operations the AllAnime client issues during
// a stream resolve: shows (search), show (episodes), episode
// (sources).
func allAnimeOKHandler(t *testing.T, hits *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		(*hits)++
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("variables")
		// Variables payload contains a "search" key on search; "_id"
		// on episodes; "showId" on sources. Use those as a coarse
		// discriminator.
		switch {
		case strings.Contains(q, "search"):
			fmt.Fprint(w, `{"data":{"shows":{"edges":[{"_id":"showXYZ","name":"Bocchi","englishName":"Bocchi","nativeName":"Bocchi","thumbnail":"","availableEpisodes":{"raw":12}}]}}}`)
		case strings.Contains(q, "showId") || strings.Contains(q, "episodeString"):
			fmt.Fprint(w, `{"data":{"episode":{"sourceUrls":[{"sourceUrl":"https://stream.example/playlist.m3u8","priority":5,"type":"hls"}]}}}`)
		default:
			fmt.Fprint(w, `{"data":{"show":{"_id":"showXYZ","availableEpisodesDetail":{"raw":["1","2","3","4"]}}}}`)
		}
	}
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

func TestRawResolver_LibraryHit_NoCache(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":100}}`)
	}))
	defer libSrv.Close()

	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})
	r := NewRawResolver(allanime.NewClient(allanime.Config{Domains: []string{"x"}}), libClient, animeRepo, cacheC, nil)

	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "library" {
		t.Errorf("Source = %q, want library", got.Source)
	}
	if !strings.HasPrefix(got.URL, "http://minio:9000/") {
		t.Errorf("URL = %q, want minio prefix", got.URL)
	}

	// Source-decision cache must be set to "library".
	var decision string
	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	if err := cacheC.Get(context.Background(), key, &decision); err != nil {
		t.Fatalf("source-decision cache: %v", err)
	}
	if decision != "library" {
		t.Errorf("source-decision = %q, want library", decision)
	}
}

func TestRawResolver_Library404_FallsThroughToAllAnime(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, libClient, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}
	// Decision cache must be set to "allanime".
	var decision string
	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	if err := cacheC.Get(context.Background(), key, &decision); err != nil {
		t.Fatalf("source-decision cache: %v", err)
	}
	if decision != "allanime" {
		t.Errorf("source-decision = %q, want allanime", decision)
	}
}

func TestRawResolver_Library5xx_FallsThrough_NoCacheWrite(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, libClient, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}

	// Decision cache must NOT be set (transient failure).
	var decision string
	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	err = cacheC.Get(context.Background(), key, &decision)
	if err == nil {
		t.Errorf("source-decision cache should be empty on 5xx, found %q", decision)
	}
}

func TestRawResolver_LibraryTimeout_FallsThrough_NoCacheWrite_Under2_5s(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep past the library client timeout (100ms in this test).
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 100 * time.Millisecond})

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, libClient, animeRepo, cacheC, nil)
	start := time.Now()
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}
	if elapsed > 2500*time.Millisecond {
		t.Errorf("total wall time %s exceeds 2.5s SLA", elapsed)
	}
	// Decision cache must NOT be set on transient error.
	var decision string
	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	if err := cacheC.Get(context.Background(), key, &decision); err == nil {
		t.Errorf("source-decision cache should be empty on timeout, found %q", decision)
	}
}

func TestRawResolver_CachedLibrary_StillHitsLibrary(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	// Pre-seed the source-decision cache.
	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	if err := cacheC.Set(context.Background(), key, "library", time.Hour); err != nil {
		t.Fatalf("seed source-decision: %v", err)
	}

	libCalls := 0
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		libCalls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio/x.m3u8","duration_sec":0,"size_bytes":0}}`)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	r := NewRawResolver(allanime.NewClient(allanime.Config{Domains: []string{"x"}}), libClient, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "library" {
		t.Errorf("Source = %q, want library", got.Source)
	}
	if libCalls != 1 {
		t.Errorf("library calls = %d, want 1 (fresh fetch on cached library decision)", libCalls)
	}
}

func TestRawResolver_CachedAllAnime_SkipsLibrary(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	key := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, testAnimeID, 1)
	if err := cacheC.Set(context.Background(), key, "allanime", time.Hour); err != nil {
		t.Fatalf("seed source-decision: %v", err)
	}

	libCalls := 0
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		libCalls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio/x.m3u8"}}`)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, libClient, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}
	if libCalls != 0 {
		t.Errorf("library calls = %d, want 0 when source-decision is allanime", libCalls)
	}
}

func TestRawResolver_NilLibraryClient_FallsToAllAnime(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, nil, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}
}

func TestRawResolver_EmptyShikimoriID_FallsToAllAnime(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(false, "")) // empty shikimori_id

	libCalls := 0
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		libCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	allCalls := 0
	aaSrv := httptest.NewServer(allAnimeOKHandler(t, &allCalls))
	defer aaSrv.Close()
	aaClient := newAllAnimeMockClient(t, aaSrv)

	r := NewRawResolver(aaClient, libClient, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 1, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime", got.Source)
	}
	if libCalls != 0 {
		t.Errorf("library calls = %d, want 0 when shikimori_id is empty", libCalls)
	}
}

func TestRawResolver_BackwardCompat_OldCachedStream_NormalizedToAllAnime(t *testing.T) {
	cacheC := newTestRedis(t)
	_, animeRepo := newTestDBWithAnime(t, makeAnime(true, testShikimoriID))

	// Pre-populate raw:stream:* WITHOUT Source (simulating a v0.1
	// cache entry from before the field existed). We encode an old
	// RawStream shape via a small local type.
	type oldRawStream struct {
		URL       string        `json:"url"`
		Type      string        `json:"type"`
		Quality   string        `json:"quality,omitempty"`
		Subtitles []RawSubtitle `json:"subtitles,omitempty"`
		ExpiresAt time.Time     `json:"expires_at"`
	}
	streamKey := fmt.Sprintf("%s:%s:%d:%s", CacheKeyStream, testAnimeID, 5, "")
	if err := cacheC.Set(context.Background(), streamKey, oldRawStream{
		URL: "https://old.example/stream.m3u8", Type: "hls", ExpiresAt: time.Now().Add(time.Hour),
	}, time.Hour); err != nil {
		t.Fatalf("seed old stream cache: %v", err)
	}

	r := NewRawResolver(allanime.NewClient(allanime.Config{Domains: []string{"x"}}), nil, animeRepo, cacheC, nil)
	got, err := r.GetStream(context.Background(), testAnimeID, 5, "")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.Source != "allanime" {
		t.Errorf("Source = %q, want allanime (normalized from empty)", got.Source)
	}
	if got.URL != "https://old.example/stream.m3u8" {
		t.Errorf("URL = %q, want carried-through from old cache", got.URL)
	}
}

func TestRawResolver_LibraryHit_SetsHasRaw(t *testing.T) {
	cacheC := newTestRedis(t)
	db, animeRepo := newTestDBWithAnime(t, makeAnime(false, testShikimoriID))

	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"minio_url":"http://minio:9000/x.m3u8","duration_sec":0,"size_bytes":0}}`)
	}))
	defer libSrv.Close()
	libClient := library.NewClient(library.Config{APIURL: libSrv.URL, Timeout: 2 * time.Second})

	r := NewRawResolver(allanime.NewClient(allanime.Config{Domains: []string{"x"}}), libClient, animeRepo, cacheC, nil)
	if _, err := r.GetStream(context.Background(), testAnimeID, 1, ""); err != nil {
		t.Fatalf("GetStream: %v", err)
	}

	// has_raw must now be 1.
	var hasRaw int
	if err := db.Raw(`SELECT has_raw FROM animes WHERE id = ?`, testAnimeID).Row().Scan(&hasRaw); err != nil {
		t.Fatalf("read has_raw: %v", err)
	}
	if hasRaw != 1 {
		t.Errorf("has_raw = %d, want 1 (lazy backfill on library hit)", hasRaw)
	}
}
