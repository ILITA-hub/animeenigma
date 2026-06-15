package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

func newTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_daily_puzzle (
		date TEXT PRIMARY KEY, anime_id TEXT, answer_snapshot TEXT, created_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_user_game_result (
		id TEXT PRIMARY KEY, user_id TEXT, puzzle_date TEXT, mode TEXT, solved INTEGER, gave_up INTEGER,
		attempts INTEGER, guesses TEXT, solved_at DATETIME, created_at DATETIME, updated_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_user_stats (
		user_id TEXT PRIMARY KEY, games_played INTEGER, games_won INTEGER, current_streak INTEGER,
		max_streak INTEGER, guess_distribution TEXT, last_played_date TEXT, updated_at DATETIME)`).Error)
	return db
}

func TestGameRepo_DailyPuzzleRoundTrip(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()

	_, err := r.GetDailyPuzzle(ctx, "2026-06-15")
	require.ErrorIs(t, err, ErrNotFound)

	p := &domain.DailyPuzzle{Date: "2026-06-15", AnimeID: "frieren",
		AnswerSnapshot: domain.Snapshot{ID: "frieren", NameRU: "Фрирен", Year: 2023}}
	require.NoError(t, r.CreateDailyPuzzle(ctx, p))

	got, err := r.GetDailyPuzzle(ctx, "2026-06-15")
	require.NoError(t, err)
	assert.Equal(t, "frieren", got.AnimeID)
	assert.Equal(t, 2023, got.AnswerSnapshot.Year)
}

func TestGameRepo_RecentAnswerIDs(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()
	for _, d := range []struct{ date, id string }{{"2026-06-13", "a"}, {"2026-06-14", "b"}, {"2026-06-15", "c"}} {
		require.NoError(t, r.CreateDailyPuzzle(ctx, &domain.DailyPuzzle{Date: d.date, AnimeID: d.id}))
	}
	ids, err := r.RecentAnswerIDs(ctx, 2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"c", "b"}, ids) // most recent 2 dates
}

func TestGameRepo_UserResultUpsertAndStats(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()

	res, err := r.GetUserResult(ctx, "u1", "2026-06-15", "daily")
	require.NoError(t, err)
	assert.Nil(t, res)

	require.NoError(t, r.SaveUserResult(ctx, &domain.UserGameResult{
		UserID: "u1", PuzzleDate: "2026-06-15", Mode: "daily", Attempts: 1, Guesses: []string{"x"}}))
	require.NoError(t, r.SaveUserResult(ctx, &domain.UserGameResult{
		UserID: "u1", PuzzleDate: "2026-06-15", Mode: "daily", Attempts: 2, Guesses: []string{"x", "y"}, Solved: true}))

	res, err = r.GetUserResult(ctx, "u1", "2026-06-15", "daily")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Solved)
	assert.Equal(t, 2, res.Attempts)
	assert.Equal(t, []string{"x", "y"}, res.Guesses)

	require.NoError(t, r.SaveUserStats(ctx, &domain.UserStats{UserID: "u1", GamesPlayed: 1, CurrentStreak: 1}))
	st, err := r.GetUserStats(ctx, "u1")
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, 1, st.CurrentStreak)
}
