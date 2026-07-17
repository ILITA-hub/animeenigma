package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
)

func newDismissalsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.RecAnnouncementDismissal{}))
	return db
}

func TestAnnouncementDismissals_InsertAndList(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Insert(ctx, "u1", "a1"))
	require.NoError(t, r.Insert(ctx, "u1", "a2"))
	require.NoError(t, r.Insert(ctx, "u2", "a3"))

	ids, err := r.ListAnimeIDs(ctx, "u1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a1", "a2"}, ids)
}

func TestAnnouncementDismissals_InsertIdempotent(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Insert(ctx, "u1", "a1"))
	require.NoError(t, r.Insert(ctx, "u1", "a1")) // duplicate — must not error

	ids, err := r.ListAnimeIDs(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"a1"}, ids)
}

func TestAnnouncementDismissals_ListEmptyForUnknownUser(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ids, err := r.ListAnimeIDs(context.Background(), "nobody")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
