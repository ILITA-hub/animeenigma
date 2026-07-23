package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAiringOccurrenceRepo(t *testing.T) *AnimeRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY, name TEXT, hidden BOOLEAN DEFAULT false, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT, name_ru TEXT)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT, PRIMARY KEY (anime_id, genre_id))`).Error)
	require.NoError(t, db.AutoMigrate(&domain.AnimeAiringOccurrence{}))
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, hidden) VALUES
		('visible', 'Visible', false), ('hidden', 'Hidden', true)`).Error)
	return NewAnimeRepository(db)
}

func TestAnimeRepository_AiringOccurrencesUpsertAndRange(t *testing.T) {
	r := setupAiringOccurrenceRepo(t)
	ctx := context.Background()
	first := time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)
	corrected := first.Add(time.Hour)
	hidden := first.Add(2 * time.Hour)

	require.NoError(t, r.UpsertAiringOccurrences(ctx, []domain.AnimeAiringOccurrence{
		{AnimeID: "visible", Episode: 9, AiredAt: first, Source: "anilist"},
		{AnimeID: "hidden", Episode: 1, AiredAt: hidden, Source: "anilist"},
	}))
	require.NoError(t, r.UpsertAiringOccurrences(ctx, []domain.AnimeAiringOccurrence{
		{AnimeID: "visible", Episode: 9, AiredAt: corrected, Source: "anilist"},
	}))

	got, err := r.GetAiringOccurrences(ctx, first, first.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "visible", got[0].AnimeID)
	require.Equal(t, 9, got[0].Episode)
	require.True(t, got[0].AiredAt.Equal(corrected))
	require.NotNil(t, got[0].Anime)
	require.Equal(t, "Visible", got[0].Anime.Name)
}
