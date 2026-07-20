package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInterestTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY, name TEXT, status TEXT, score REAL,
		episodes_aired INTEGER DEFAULT 0, next_episode_at DATETIME,
		hidden INTEGER DEFAULT 0, sort_priority INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		user_id TEXT, anime_id TEXT, status TEXT
	)`).Error)
	return db
}

func seedInterestAnime(t *testing.T, db *gorm.DB, id, name, status string, score float64, aired, sortPri int, hidden bool) {
	t.Helper()
	h := 0
	if hidden {
		h = 1
	}
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id,name,status,score,episodes_aired,sort_priority,hidden) VALUES (?,?,?,?,?,?,?)`,
		id, name, status, score, aired, sortPri, h).Error)
}

func TestListInterestBands(t *testing.T) {
	db := setupInterestTestDB(t)
	// ongoing (2), released top-eligible (3), one hidden (excluded everywhere).
	seedInterestAnime(t, db, "ong1", "Ongoing High", "ongoing", 8.5, 12, 0, false)
	seedInterestAnime(t, db, "ong2", "Ongoing Low", "ongoing", 6.0, 3, 0, false)
	seedInterestAnime(t, db, "rel1", "Released A", "released", 9.1, 24, 5, false)
	seedInterestAnime(t, db, "rel2", "Released B", "released", 7.0, 12, 0, false)
	seedInterestAnime(t, db, "rel3", "Released C", "released", 6.5, 24, 0, false)
	seedInterestAnime(t, db, "hid1", "Hidden", "released", 9.9, 1, 9, true)
	// planners: rel2 has 2 plan_to_watch, rel3 has 1.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (user_id,anime_id,status) VALUES
		('u1','rel2','plan_to_watch'),('u2','rel2','plan_to_watch'),('u3','rel3','plan_to_watch'),
		('u4','rel2','watching')`).Error)

	r := &AnimeRepository{db: db}
	b, err := r.ListInterestBands(context.Background(), 500, 2, 2, 0)
	require.NoError(t, err)

	// Ongoing: only status=ongoing, score DESC, hidden excluded.
	require.Len(t, b.Ongoing, 2)
	require.Equal(t, "ong1", b.Ongoing[0].ID)

	// Top: browse order sort_priority DESC, score DESC, hidden excluded, LIMIT 2.
	// rel1 (sort_priority 5) first, then highest score among the rest = ong1 (8.5).
	require.Len(t, b.Top, 2)
	require.Equal(t, "rel1", b.Top[0].ID)
	require.Equal(t, 1, b.Top[0].TopRank)
	require.Equal(t, 2, b.Top[1].TopRank)

	// Planned: non-ongoing with plan_to_watch, planners DESC, LIMIT 2.
	require.Len(t, b.Planned, 2)
	require.Equal(t, "rel2", b.Planned[0].ID)
	require.Equal(t, 2, b.Planned[0].Planners)

	// IdleWindow: non-ongoing browse order, OFFSET 0 LIMIT 2 → rel1, rel3 (rel1 sort_priority 5 first, then score DESC rel3 6.5 > rel2 7.0? rel2=7.0 > rel3=6.5, so rel1, rel2).
	require.Len(t, b.IdleWindow, 2)
	require.Equal(t, "rel1", b.IdleWindow[0].ID)
	require.Equal(t, "rel2", b.IdleWindow[1].ID)

	// IdleTotal: all visible non-ongoing = rel1, rel2, rel3 = 3.
	require.Equal(t, 3, b.IdleTotal)
}
