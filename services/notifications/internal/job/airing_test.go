package job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAiringTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (id TEXT PRIMARY KEY, next_episode_at DATETIME)`).Error)
	return db
}

func TestAiringTimes(t *testing.T) {
	db := setupAiringTestDB(t)
	soon := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(`INSERT INTO animes (id,next_episode_at) VALUES ('a1',?),('a2',NULL)`, soon).Error)

	c := NewHotCombosCollector(db, nil)
	got, err := c.AiringTimes(context.Background(), []string{"a1", "a2", "a3"})
	require.NoError(t, err)
	require.NotNil(t, got["a1"])
	require.True(t, got["a1"].Equal(soon))
	// a2 has NULL, a3 absent → both omitted from the map.
	require.Nil(t, got["a2"])
	require.NotContains(t, got, "a3")
}

func TestAiringTimesEmptyInput(t *testing.T) {
	db := setupAiringTestDB(t)
	c := NewHotCombosCollector(db, nil)
	got, err := c.AiringTimes(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
