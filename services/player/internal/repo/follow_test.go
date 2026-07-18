package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFollowTestRepo(t *testing.T) *FollowRepository {
	t.Helper()
	db := setupTestDB(t)
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY, username TEXT, public_id TEXT, avatar TEXT, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.AutoMigrate(&domain.UserFollow{}))
	return NewFollowRepository(db)
}

func TestFollowRepository_FollowIsIdempotentAndListsProfiles(t *testing.T) {
	r := setupFollowTestRepo(t)
	ctx := context.Background()
	require.NoError(t, r.db.Exec(`INSERT INTO users (id, username, public_id, avatar)
		VALUES ('target', 'alice', 'alice-public', '/alice.png')`).Error)

	require.NoError(t, r.Follow(ctx, "viewer", "target"))
	require.NoError(t, r.Follow(ctx, "viewer", "target"))

	following, err := r.IsFollowing(ctx, "viewer", "target")
	require.NoError(t, err)
	assert.True(t, following)
	users, err := r.List(ctx, "viewer")
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "alice-public", users[0].PublicID)

	require.NoError(t, r.Unfollow(ctx, "viewer", "target"))
	following, err = r.IsFollowing(ctx, "viewer", "target")
	require.NoError(t, err)
	assert.False(t, following)
}
