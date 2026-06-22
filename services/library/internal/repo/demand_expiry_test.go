package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDemandExpiryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE autocache_demand (
		mal_id TEXT NOT NULL,
		episode INTEGER NOT NULL,
		reason TEXT NOT NULL,
		requested_at DATETIME NOT NULL,
		titles TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (mal_id, episode)
	)`).Error)
	return db
}

// TestDeleteExpired only removes rows older than the cutoff (audit #20).
func TestDeleteExpired(t *testing.T) {
	db := setupDemandExpiryDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	now := time.Now()
	rows := []struct {
		mal string
		ep  int
		at  time.Time
	}{
		{"100", 1, now.Add(-30 * 24 * time.Hour)}, // stale → deleted
		{"100", 2, now.Add(-20 * 24 * time.Hour)}, // stale → deleted
		{"200", 1, now.Add(-2 * 24 * time.Hour)},  // fresh → kept
		{"300", 1, now},                           // fresh → kept
	}
	for _, x := range rows {
		require.NoError(t, db.Exec(
			`INSERT INTO autocache_demand (mal_id, episode, reason, requested_at, titles) VALUES (?,?,?,?,?)`,
			x.mal, x.ep, "ongoing", x.at, "").Error)
	}

	deleted, err := r.DeleteExpired(ctx, now.Add(-14*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted, "only the two >14d-old rows should be deleted")

	var remaining int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM autocache_demand`).Scan(&remaining).Error)
	require.Equal(t, int64(2), remaining, "the two fresh rows must survive")
}
