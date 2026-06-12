package service

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isNotFoundErr(err error) bool {
	var appErr *apperrors.AppError
	return errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound
}

// Rewatch must refuse when there is no completed entry — otherwise the
// watch_progress reset would wipe an in-flight first watch.

func TestRewatch_NotCompleted_RefusesAndKeepsProgress(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "watching", 0)
	require.NoError(t, db.Exec(
		`INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, watch_count, last_watched_at, created_at, updated_at)
		 VALUES ('wp-1', 'u1', 'anime-1', 3, 500, 1400, true, 1, now(), now(), now())`).Error)

	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.Error(t, err, "rewatching a non-completed entry must refuse")
	assert.True(t, isNotFoundErr(err))

	var completed bool
	require.NoError(t, db.Raw(
		`SELECT completed FROM watch_progress WHERE id = 'wp-1'`).Scan(&completed).Error)
	assert.True(t, completed, "in-flight watch_progress must not be wiped")
}

func TestRewatch_NoEntryAtAll_Refuses(t *testing.T) {
	svc, _ := setupListServiceTestDB(t)
	_, err := svc.Rewatch(context.Background(), "u1", "missing-anime")
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}
