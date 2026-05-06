package signals

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS3TestDB creates an in-memory SQLite DB with the minimal schema S3
// needs: animes (so the orchestrator-friendly upsert path works), watch_history
// (for the GROUP BY), and rec_population_signals (for Score read-back). All
// columns match production shape but with engine-portable types.
func setupS3TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE watch_history (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		episode_number INTEGER NOT NULL,
		player TEXT NOT NULL,
		language TEXT NOT NULL,
		watch_type TEXT NOT NULL,
		translation_id TEXT,
		translation_title TEXT,
		duration_watched INTEGER DEFAULT 0,
		session_id TEXT,
		watched_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rec_population_signals (
		anime_id TEXT PRIMARY KEY,
		s3_trending_score REAL NOT NULL DEFAULT 0,
		s4_recency_score REAL NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL
	)`).Error)
	return db
}

func seedAnimeRow(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden) VALUES (?, 0)`, id,
	).Error)
}

func seedWatchHistory(t *testing.T, db *gorm.DB, id, userID, animeID string, watchedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, episode_number, player, language, watch_type, watched_at)
		 VALUES (?, ?, ?, 1, 'kodik', 'ru', 'sub', ?)`,
		id, userID, animeID, watchedAt,
	).Error)
}

func TestS3Trending_ID(t *testing.T) {
	db := setupS3TestDB(t)
	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	assert.Equal(t, recs.SignalID("s3"), s3.ID())
}

func TestS3Trending_PrecomputeWritesDistinctUserCount(t *testing.T) {
	db := setupS3TestDB(t)
	now := time.Now().UTC()

	seedAnimeRow(t, db, "anime-A")
	seedAnimeRow(t, db, "anime-B")

	// 3 distinct users on anime-A; one of them watched twice (still counts as 1)
	seedWatchHistory(t, db, "wh1", "user-1", "anime-A", now.Add(-1*24*time.Hour))
	seedWatchHistory(t, db, "wh2", "user-1", "anime-A", now.Add(-2*time.Hour))
	seedWatchHistory(t, db, "wh3", "user-2", "anime-A", now.Add(-3*24*time.Hour))
	seedWatchHistory(t, db, "wh4", "user-3", "anime-A", now.Add(-5*24*time.Hour))
	// 2 distinct users on anime-B
	seedWatchHistory(t, db, "wh5", "user-1", "anime-B", now.Add(-7*24*time.Hour))
	seedWatchHistory(t, db, "wh6", "user-4", "anime-B", now.Add(-10*24*time.Hour))

	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	require.NoError(t, s3.Precompute(context.Background(), ""))

	var rows []domain.RecPopulationSignals
	require.NoError(t, db.Find(&rows).Error)

	scoreByID := map[string]float32{}
	for _, r := range rows {
		scoreByID[r.AnimeID] = r.S3TrendingScore
	}
	assert.Equal(t, float32(3), scoreByID["anime-A"], "DISTINCT user count for anime-A")
	assert.Equal(t, float32(2), scoreByID["anime-B"], "DISTINCT user count for anime-B")
}

func TestS3Trending_PrecomputeIgnoresOlderThan30Days(t *testing.T) {
	db := setupS3TestDB(t)
	now := time.Now().UTC()

	seedAnimeRow(t, db, "anime-A")
	seedWatchHistory(t, db, "wh-recent", "user-1", "anime-A", now.Add(-1*24*time.Hour))
	seedWatchHistory(t, db, "wh-old", "user-2", "anime-A", now.Add(-60*24*time.Hour))

	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	require.NoError(t, s3.Precompute(context.Background(), ""))

	var row domain.RecPopulationSignals
	require.NoError(t, db.Where("anime_id = ?", "anime-A").First(&row).Error)
	assert.Equal(t, float32(1), row.S3TrendingScore, "only the within-30d row contributes")
}

func TestS3Trending_PrecomputeOnEmptyHistoryNoCrash(t *testing.T) {
	db := setupS3TestDB(t)
	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	err := s3.Precompute(context.Background(), "")
	assert.NoError(t, err, "empty watch_history must not error")
}

func TestS3Trending_ScoreReadsFromPopulationSignals(t *testing.T) {
	db := setupS3TestDB(t)
	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO rec_population_signals (anime_id, s3_trending_score, s4_recency_score, last_computed)
		 VALUES (?, ?, 0, ?)`,
		"anime-A", float32(7), now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO rec_population_signals (anime_id, s3_trending_score, s4_recency_score, last_computed)
		 VALUES (?, ?, 0, ?)`,
		"anime-B", float32(2), now,
	).Error)

	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	got, err := s3.Score(context.Background(), "", []recs.AnimeID{"anime-A", "anime-B"})
	require.NoError(t, err)
	assert.Equal(t, recs.RawScore(7), got["anime-A"])
	assert.Equal(t, recs.RawScore(2), got["anime-B"])
}

func TestS3Trending_ScoreEmptyCandidates(t *testing.T) {
	db := setupS3TestDB(t)
	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	got, err := s3.Score(context.Background(), "", []recs.AnimeID{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS3Trending_ScoreOmitsAnimeWithNoSignalRow(t *testing.T) {
	db := setupS3TestDB(t)
	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO rec_population_signals (anime_id, s3_trending_score, s4_recency_score, last_computed)
		 VALUES (?, ?, 0, ?)`,
		"anime-A", float32(3), now,
	).Error)

	repo := repo.NewRecsRepository(db)
	s3 := NewS3Trending(db, repo)
	got, err := s3.Score(context.Background(), "", []recs.AnimeID{"anime-A", "anime-NEW"})
	require.NoError(t, err)
	assert.Equal(t, recs.RawScore(3), got["anime-A"])
	_, present := got["anime-NEW"]
	assert.False(t, present, "anime with no rec_population_signals row must be omitted (normalizer treats absent as zero)")
}
