package service

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPrefDirtyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE user_anime_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		player TEXT, language TEXT, watch_type TEXT,
		translation_id TEXT, translation_title TEXT,
		updated_at DATETIME
	)`).Error)
	// OnConflict(user_id, anime_id) requires this unique index.
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX ux_pref ON user_anime_preferences(user_id, anime_id)`).Error)
	return db
}

// TestUpsertAnimePreference_DirtyCheck proves the heartbeat write-amp fix
// (#13): an unchanged combo skips the write (updated_at stays put), a changed
// combo writes through.
func TestUpsertAnimePreference_DirtyCheck(t *testing.T) {
	db := setupPrefDirtyDB(t)
	svc := NewPreferenceService(repo.NewPreferenceRepository(db), logger.Default())
	ctx := context.Background()

	req := &domain.UpdateProgressRequest{
		AnimeID: "anime-1", Player: "kodik", Language: "ru",
		WatchType: "sub", TranslationID: "963", TranslationTitle: "AniLibria",
	}
	svc.UpsertAnimePreference(ctx, "user-1", req)

	// Stamp updated_at to a sentinel far in the past.
	sentinel := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(`UPDATE user_anime_preferences SET updated_at = ? WHERE user_id = ? AND anime_id = ?`,
		sentinel, "user-1", "anime-1").Error)

	// Same combo again → must be skipped (updated_at unchanged).
	svc.UpsertAnimePreference(ctx, "user-1", req)
	var got domain.UserAnimePreference
	require.NoError(t, db.Where("user_id = ? AND anime_id = ?", "user-1", "anime-1").First(&got).Error)
	require.WithinDuration(t, sentinel, got.UpdatedAt, time.Second,
		"same-combo heartbeat must skip the write (updated_at unchanged)")

	// Changed combo → must write through.
	changed := *req
	changed.WatchType = "dub"
	svc.UpsertAnimePreference(ctx, "user-1", &changed)
	require.NoError(t, db.Where("user_id = ? AND anime_id = ?", "user-1", "anime-1").First(&got).Error)
	require.True(t, got.UpdatedAt.After(sentinel), "changed combo must write (updated_at advances)")
	require.Equal(t, "dub", got.WatchType)
}
