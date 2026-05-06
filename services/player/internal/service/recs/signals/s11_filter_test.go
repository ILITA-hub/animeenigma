package signals

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupS11TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	return db
}

func TestS11Filter_EmptyTable(t *testing.T) {
	db := setupS11TestDB(t)
	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS11Filter_ExcludesHiddenAndSoftDeleted(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('visible-1', 0, NULL),
		 ('visible-2', 0, NULL),
		 ('hidden-1', 1, NULL),
		 ('deleted-1', 0, '2026-01-01 00:00:00'),
		 ('hidden-and-deleted-1', 1, '2026-01-01 00:00:00')`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)

	want := map[recs.AnimeID]struct{}{"visible-1": {}, "visible-2": {}}
	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	assert.Equal(t, want, gotSet, "S11 must include hidden=false AND deleted_at IS NULL only")
}

func TestS11Filter_AllVisible(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('a', 0, NULL),
		 ('b', 0, NULL),
		 ('c', 0, NULL)`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 3)
}
