// Tests for the BackfillRunner — sqlite in-memory fixture with mock
// shikimori / anilist / arm fetchers. Mirrors the test pattern in
// services/catalog/internal/domain/anime_attributes_test.go (postgres-only
// `default:gen_random_uuid()` clauses are pre-created via raw SQL so
// AutoMigrate skips them; the new Phase-12 tables are AutoMigrated).
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	loggerlib "github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	catalogdomain "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist"
)

// registerSQLiteUDFsOnce installs a SQLite driver named
// "sqlite3_backfill_attrs" with a gen_random_uuid() UDF. Without this,
// CREATE TABLE animes (which carries `default:gen_random_uuid()`) chokes.
var registerSQLiteUDFsOnce sync.Once

func registerSQLiteUDFs() {
	registerSQLiteUDFsOnce.Do(func() {
		sql.Register("sqlite3_backfill_attrs", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("gen_random_uuid", func() string {
					b := make([]byte, 16)
					_, _ = rand.Read(b)
					return hex.EncodeToString(b)
				}, true)
			},
		})
	})
}

// newTestDB returns an in-memory SQLite gorm.DB pre-populated with the
// Phase-12 schema (Anime + Genre + Studio + Tag + AnimeTag + the m2m
// join tables). Each test gets a fresh DB.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteUDFs()

	rawDB, err := sql.Open("sqlite3_backfill_attrs", ":memory:")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite3_backfill_attrs",
		Conn:       rawDB,
	}, &gorm.Config{})
	require.NoError(t, err)

	rawSQL := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			name_jp TEXT,
			description TEXT,
			year INTEGER,
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
			ani_list_id TEXT,
			has_video INTEGER DEFAULT 0,
			hidden INTEGER DEFAULT 0,
			sort_priority INTEGER DEFAULT 0,
			next_episode_at DATETIME,
			aired_on DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE anime_genres (
			anime_id TEXT NOT NULL,
			genre_id TEXT NOT NULL,
			PRIMARY KEY (anime_id, genre_id)
		)`,
		`CREATE TABLE anime_studios (
			anime_id TEXT NOT NULL,
			studio_id TEXT NOT NULL,
			PRIMARY KEY (anime_id, studio_id)
		)`,
		`CREATE TABLE anime_tags (
			anime_id TEXT NOT NULL,
			tag_id TEXT NOT NULL,
			rank INTEGER DEFAULT 0,
			created_at DATETIME,
			PRIMARY KEY (anime_id, tag_id)
		)`,
	}
	for _, ddl := range rawSQL {
		require.NoError(t, db.Exec(ddl).Error)
	}

	require.NoError(t, db.AutoMigrate(
		&catalogdomain.Anime{},
		&catalogdomain.Genre{},
		&catalogdomain.Studio{},
		&catalogdomain.Tag{},
		&catalogdomain.AnimeTag{},
	))
	require.NoError(t, db.SetupJoinTable(&catalogdomain.Anime{}, "Tags", &catalogdomain.AnimeTag{}))
	return db
}

func newTestLogger(t *testing.T) *loggerlib.Logger {
	t.Helper()
	l, err := loggerlib.New(loggerlib.Config{Level: "warn", Encoding: "console"})
	require.NoError(t, err)
	return l
}

// insertAnime is a helper for creating fixture rows quickly.
func insertAnime(t *testing.T, db *gorm.DB, a catalogdomain.Anime) {
	t.Helper()
	if a.ID == "" {
		a.ID = "anime-" + hexToken()
	}
	if a.Name == "" {
		a.Name = "Test " + a.ID
	}
	if a.ShikimoriID == "" {
		a.ShikimoriID = "sk-" + a.ID
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
		a.UpdatedAt = a.CreatedAt
	}
	require.NoError(t, db.Create(&a).Error)
}

func hexToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// --- Mock fetchers ----------------------------------------------------

type mockShikimori struct {
	mu       sync.Mutex
	calls    []string // shikimoriIDs called, in order
	response *catalogdomain.Anime
	errBy    map[string]error // shikimoriID -> error
}

func (m *mockShikimori) GetAnimeByID(_ context.Context, shikimoriID string) (*catalogdomain.Anime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, shikimoriID)
	if err, ok := m.errBy[shikimoriID]; ok {
		return nil, err
	}
	if m.response == nil {
		return nil, errors.New("mock: no response configured")
	}
	cp := *m.response
	return &cp, nil
}

func (m *mockShikimori) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type mockAnilist struct {
	mu       sync.Mutex
	calls    []int // anilistIDs called
	tagsByID map[int][]anilist.Tag
	errBy    map[int]error
}

func (m *mockAnilist) FetchTags(_ context.Context, anilistID int) ([]anilist.Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, anilistID)
	if err, ok := m.errBy[anilistID]; ok {
		return nil, err
	}
	if tags, ok := m.tagsByID[anilistID]; ok {
		return tags, nil
	}
	return []anilist.Tag{}, nil
}

func (m *mockAnilist) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type mockARM struct {
	mu        sync.Mutex
	calls     []string
	resultBy  map[string]*idmapping.MappingResult // shikimoriID -> result; nil = 404
	errBy     map[string]error
}

func (m *mockARM) ResolveByShikimoriID(id string) (*idmapping.MappingResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, id)
	if err, ok := m.errBy[id]; ok {
		return nil, err
	}
	if r, ok := m.resultBy[id]; ok {
		return r, nil
	}
	return nil, nil // default: no mapping found (404)
}

// helper to build a *int for AniList field
func intp(v int) *int { return &v }

// --- ShikimoriHalf tests ----------------------------------------------

func TestBackfillRunner_ShikimoriHalf_PopulatesNewColumns(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "a1", ShikimoriID: "1001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "a2", ShikimoriID: "1002"})
	insertAnime(t, db, catalogdomain.Anime{ID: "a3", ShikimoriID: "1003"})

	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind:           "tv",
		Rating:         "pg_13",
		MaterialSource: "manga",
		Studios:        []catalogdomain.Studio{{ID: "11", Name: "Madhouse"}},
	}}
	al := &mockAnilist{}
	arm := &mockARM{}

	r := NewBackfillRunner(db, sh, al, arm, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, res.Succeeded)
	assert.Equal(t, 0, res.Skipped)
	assert.Equal(t, 0, res.Failed)

	// Confirm rows updated
	var rows []catalogdomain.Anime
	require.NoError(t, db.Find(&rows).Error)
	for _, row := range rows {
		assert.Equal(t, "tv", row.Kind, "row %s should have kind populated", row.ID)
		assert.Equal(t, "pg_13", row.Rating)
		assert.Equal(t, "manga", row.MaterialSource)
	}

	// Studios upserted exactly once across 3 anime
	var studioCount int64
	require.NoError(t, db.Model(&catalogdomain.Studio{}).Count(&studioCount).Error)
	assert.Equal(t, int64(1), studioCount, "Madhouse must be deduped to 1 studio row")

	// 3 join rows in anime_studios
	var joinCount int64
	require.NoError(t, db.Table("anime_studios").Count(&joinCount).Error)
	assert.Equal(t, int64(3), joinCount)
}

func TestBackfillRunner_ShikimoriHalf_SkipsAlreadyPopulated(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{
		ID:             "done1",
		ShikimoriID:    "2001",
		Kind:           "tv",
		Rating:         "pg_13",
		MaterialSource: "manga",
	})
	// Pre-create a studio row + join so the NOT EXISTS check trips
	require.NoError(t, db.Create(&catalogdomain.Studio{ID: "99", Name: "BONES"}).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES (?, ?)`,
		"done1", "99").Error)

	insertAnime(t, db, catalogdomain.Anime{
		ID:             "done2",
		ShikimoriID:    "2002",
		Kind:           "movie",
		Rating:         "g",
		MaterialSource: "original",
	})
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES (?, ?)`,
		"done2", "99").Error)

	insertAnime(t, db, catalogdomain.Anime{ID: "todo", ShikimoriID: "2003"})

	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
		Studios: []catalogdomain.Studio{{ID: "12", Name: "MAPPA"}},
	}}
	r := NewBackfillRunner(db, sh, &mockAnilist{}, &mockARM{}, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Succeeded)
	assert.Equal(t, 0, res.Failed)

	// fetcher must have been called exactly once (only for the "todo" row)
	assert.Equal(t, 1, sh.callCount(), "already-populated rows must not trigger Shikimori fetches")
	assert.Equal(t, []string{"2003"}, sh.calls)
}

func TestBackfillRunner_ShikimoriHalf_ContinuesOnFetchError(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "x1", ShikimoriID: "3001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "x2", ShikimoriID: "3002"})
	insertAnime(t, db, catalogdomain.Anime{ID: "x3", ShikimoriID: "3003"})

	sh := &mockShikimori{
		response: &catalogdomain.Anime{
			Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
		},
		errBy: map[string]error{"3002": errors.New("upstream 502")},
	}
	r := NewBackfillRunner(db, sh, &mockAnilist{}, &mockARM{}, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err, "per-anime errors must NOT bubble up as a run error")
	assert.Equal(t, 2, res.Succeeded)
	assert.Equal(t, 1, res.Failed)

	// x1 + x3 should be populated; x2 stays empty
	var x1, x2, x3 catalogdomain.Anime
	require.NoError(t, db.First(&x1, "id = ?", "x1").Error)
	require.NoError(t, db.First(&x2, "id = ?", "x2").Error)
	require.NoError(t, db.First(&x3, "id = ?", "x3").Error)
	assert.Equal(t, "tv", x1.Kind)
	assert.Equal(t, "", x2.Kind, "errored row must not be partially written")
	assert.Equal(t, "tv", x3.Kind)
}

func TestBackfillRunner_ShikimoriHalf_PartialFieldsFromShikimori(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "p1", ShikimoriID: "4001"})

	// Shikimori returns Kind only — Rating + MaterialSource are empty
	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind:    "tv",
		Studios: []catalogdomain.Studio{{ID: "1", Name: "Madhouse"}},
	}}
	r := NewBackfillRunner(db, sh, &mockAnilist{}, &mockARM{}, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Succeeded)

	var got catalogdomain.Anime
	require.NoError(t, db.First(&got, "id = ?", "p1").Error)
	assert.Equal(t, "tv", got.Kind)
	assert.Equal(t, "", got.Rating, "Rating empty when Shikimori has none — S5 contributes zero")
	assert.Equal(t, "", got.MaterialSource)
}

func TestBackfillRunner_ShikimoriHalf_StudiosUpsert(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "s1", ShikimoriID: "5001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "s2", ShikimoriID: "5002"})

	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
		Studios: []catalogdomain.Studio{{ID: "1", Name: "Madhouse"}},
	}}
	r := NewBackfillRunner(db, sh, &mockAnilist{}, &mockARM{}, newTestLogger(t), Config{LogEvery: 100})
	_, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)

	var studioCount int64
	require.NoError(t, db.Model(&catalogdomain.Studio{}).Count(&studioCount).Error)
	assert.Equal(t, int64(1), studioCount, "1 dedup'd studio across 2 anime")

	var joinCount int64
	require.NoError(t, db.Table("anime_studios").Count(&joinCount).Error)
	assert.Equal(t, int64(2), joinCount, "2 join rows (one per anime)")
}

// --- AnilistHalf tests ------------------------------------------------

func TestBackfillRunner_AnilistHalf_PopulatesTags(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "t1", ShikimoriID: "6001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "t2", ShikimoriID: "6002"})

	arm := &mockARM{resultBy: map[string]*idmapping.MappingResult{
		"6001": {AniList: intp(12345)},
		"6002": {AniList: nil}, // no AniList mapping
	}}
	al := &mockAnilist{tagsByID: map[int][]anilist.Tag{
		12345: {
			{Name: "Slice of Life", Rank: 85},
			{Name: "School", Rank: 70},
		},
	}}

	r := NewBackfillRunner(db, &mockShikimori{}, al, arm, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Succeeded)
	assert.Equal(t, 1, res.SkippedNoAnilist)
	assert.Equal(t, 0, res.Failed)

	var tagCount int64
	require.NoError(t, db.Model(&catalogdomain.Tag{}).Count(&tagCount).Error)
	assert.Equal(t, int64(2), tagCount)

	var joinCount int64
	require.NoError(t, db.Table("anime_tags").Count(&joinCount).Error)
	assert.Equal(t, int64(2), joinCount, "2 join rows for t1, 0 for t2")

	// Confirm ranks persisted
	var sliceRank int
	require.NoError(t, db.Raw(`SELECT rank FROM anime_tags WHERE anime_id = ? AND tag_id = ?`,
		"t1", "slice_of_life").Scan(&sliceRank).Error)
	assert.Equal(t, 85, sliceRank)
}

func TestBackfillRunner_AnilistHalf_SkipsAlreadyTagged(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "tagged", ShikimoriID: "7001"})
	require.NoError(t, db.Create(&catalogdomain.Tag{ID: "existing", Name: "Existing"}).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_tags (anime_id, tag_id, rank, created_at) VALUES (?, ?, ?, ?)`,
		"tagged", "existing", 50, time.Now()).Error)

	arm := &mockARM{resultBy: map[string]*idmapping.MappingResult{
		"7001": {AniList: intp(99)},
	}}
	al := &mockAnilist{tagsByID: map[int][]anilist.Tag{
		99: {{Name: "Should Not Be Inserted", Rank: 100}},
	}}
	r := NewBackfillRunner(db, &mockShikimori{}, al, arm, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res.Succeeded)
	assert.Equal(t, 0, res.SkippedNoAnilist)
	assert.Equal(t, 0, res.Failed)

	// arm and anilist must NOT have been called for the already-tagged anime
	assert.Equal(t, 0, len(arm.calls), "arm must not be called for already-tagged rows")
	assert.Equal(t, 0, al.callCount(), "anilist must not be called for already-tagged rows")
}

func TestBackfillRunner_AnilistHalf_TagDedupAcrossAnime(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "d1", ShikimoriID: "8001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "d2", ShikimoriID: "8002"})

	arm := &mockARM{resultBy: map[string]*idmapping.MappingResult{
		"8001": {AniList: intp(111)},
		"8002": {AniList: intp(222)},
	}}
	al := &mockAnilist{tagsByID: map[int][]anilist.Tag{
		111: {{Name: "Slice of Life", Rank: 85}},
		222: {{Name: "Slice of Life", Rank: 60}},
	}}
	r := NewBackfillRunner(db, &mockShikimori{}, al, arm, newTestLogger(t), Config{LogEvery: 100})
	_, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)

	var tagCount int64
	require.NoError(t, db.Model(&catalogdomain.Tag{}).Count(&tagCount).Error)
	assert.Equal(t, int64(1), tagCount, "dedup'd to 1 tag row")

	var joinCount int64
	require.NoError(t, db.Table("anime_tags").Count(&joinCount).Error)
	assert.Equal(t, int64(2), joinCount)

	// Verify ranks per anime
	var d1Rank, d2Rank int
	require.NoError(t, db.Raw(`SELECT rank FROM anime_tags WHERE anime_id = ?`, "d1").Scan(&d1Rank).Error)
	require.NoError(t, db.Raw(`SELECT rank FROM anime_tags WHERE anime_id = ?`, "d2").Scan(&d2Rank).Error)
	assert.Equal(t, 85, d1Rank)
	assert.Equal(t, 60, d2Rank)
}

func TestBackfillRunner_AnilistHalf_ContinuesOnAnilistError(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "e1", ShikimoriID: "9001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "e2", ShikimoriID: "9002"})

	arm := &mockARM{resultBy: map[string]*idmapping.MappingResult{
		"9001": {AniList: intp(101)},
		"9002": {AniList: intp(102)},
	}}
	al := &mockAnilist{
		tagsByID: map[int][]anilist.Tag{
			102: {{Name: "Action", Rank: 90}},
		},
		errBy: map[int]error{101: errors.New("anilist 500")},
	}
	r := NewBackfillRunner(db, &mockShikimori{}, al, arm, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Succeeded)
	assert.Equal(t, 1, res.Failed)
}

func TestBackfillRunner_AnilistHalf_ContinuesOnArmError(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "ae", ShikimoriID: "10001"})

	arm := &mockARM{errBy: map[string]error{"10001": errors.New("arm down")}}
	r := NewBackfillRunner(db, &mockShikimori{}, &mockAnilist{}, arm, newTestLogger(t), Config{LogEvery: 100})
	res, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

// --- Shared flag tests ------------------------------------------------

func TestBackfillRunner_LimitFlag(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 5; i++ {
		insertAnime(t, db, catalogdomain.Anime{
			ID:          fmt.Sprintf("lim-%d", i),
			ShikimoriID: fmt.Sprintf("L%d", 1000+i),
		})
	}
	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
	}}
	r := NewBackfillRunner(db, sh, &mockAnilist{}, &mockARM{}, newTestLogger(t),
		Config{Limit: 2, LogEvery: 100})
	res, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Succeeded, "limit=2 caps the work")
	assert.Equal(t, 2, sh.callCount())
}

func TestBackfillRunner_DryRun(t *testing.T) {
	db := newTestDB(t)
	insertAnime(t, db, catalogdomain.Anime{ID: "dr1", ShikimoriID: "20001"})
	insertAnime(t, db, catalogdomain.Anime{ID: "dr2", ShikimoriID: "20002"})

	sh := &mockShikimori{response: &catalogdomain.Anime{
		Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
		Studios: []catalogdomain.Studio{{ID: "1", Name: "Madhouse"}},
	}}
	arm := &mockARM{resultBy: map[string]*idmapping.MappingResult{
		"20001": {AniList: intp(1)},
		"20002": {AniList: intp(2)},
	}}
	al := &mockAnilist{tagsByID: map[int][]anilist.Tag{
		1: {{Name: "Action"}},
		2: {{Name: "Drama"}},
	}}

	r := NewBackfillRunner(db, sh, al, arm, newTestLogger(t),
		Config{DryRun: true, LogEvery: 100})
	shRes, err := r.ShikimoriHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, shRes.Succeeded, "dry-run still reports what WOULD happen")

	alRes, err := r.AnilistHalf(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, alRes.Succeeded)

	// Confirm NO writes happened
	var got1, got2 catalogdomain.Anime
	require.NoError(t, db.First(&got1, "id = ?", "dr1").Error)
	require.NoError(t, db.First(&got2, "id = ?", "dr2").Error)
	assert.Equal(t, "", got1.Kind, "dry-run must not write Kind")
	assert.Equal(t, "", got2.Kind)

	var studioCount, tagCount, animeStudios, animeTags int64
	require.NoError(t, db.Model(&catalogdomain.Studio{}).Count(&studioCount).Error)
	require.NoError(t, db.Model(&catalogdomain.Tag{}).Count(&tagCount).Error)
	require.NoError(t, db.Table("anime_studios").Count(&animeStudios).Error)
	require.NoError(t, db.Table("anime_tags").Count(&animeTags).Error)
	assert.Equal(t, int64(0), studioCount, "dry-run must not write studios")
	assert.Equal(t, int64(0), tagCount, "dry-run must not write tags")
	assert.Equal(t, int64(0), animeStudios)
	assert.Equal(t, int64(0), animeTags)
}
