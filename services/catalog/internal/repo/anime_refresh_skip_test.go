package repo

// Tests for the batch-refresh "skip unchanged rows" support added to the
// AnimeRepository: the AnimeMetadataEqual field comparison (which lets the
// service skip a no-op Update) and TouchUpdatedAt (which advances only
// updated_at for unchanged rows so they still leave the stale-refresh window
// without a full-row rewrite). Reuses setupAnimeUpdateTestDB / seedExistingAnime
// from anime_update_test.go (same package).

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnimeMetadataEqual(t *testing.T) {
	next := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	nextOther := next
	aired := time.Date(2020, 4, 5, 0, 0, 0, 0, time.UTC)

	base := func() *domain.Anime {
		n := next
		a := aired
		return &domain.Anime{
			Name: "Frieren", NameEN: "Frieren", NameRU: "Фрирен", NameJP: "葬送のフリーレン",
			Description: "desc", Year: 2023, Season: "fall", Status: domain.StatusReleased,
			Kind: "tv", Rating: "pg_13", MaterialSource: "manga",
			EpisodesCount: 28, EpisodesAired: 28, EpisodeDuration: 24,
			Score: 9.02, PosterURL: "http://p/1.jpg",
			NextEpisodeAt: &n, AiredOn: &a,
		}
	}

	t.Run("identical is equal", func(t *testing.T) {
		a, b := base(), base()
		// Distinct pointers, same instant — must still compare equal.
		b.NextEpisodeAt = &nextOther
		assert.True(t, AnimeMetadataEqual(a, b))
	})

	t.Run("lazy/admin columns are ignored", func(t *testing.T) {
		a, b := base(), base()
		// Fields NOT in animeMetadataColumns must not affect equality.
		b.HasVideo = true
		b.SortPriority = 5
		b.Hidden = true
		b.MALID = "999"
		b.HasEnglish = true
		assert.True(t, AnimeMetadataEqual(a, b))
	})

	t.Run("score equal within decimal(4,2) precision", func(t *testing.T) {
		a, b := base(), base()
		a.Score = 9.02
		b.Score = 9.024 // rounds to 9.02 when stored
		assert.True(t, AnimeMetadataEqual(a, b))
	})

	t.Run("real score change is detected", func(t *testing.T) {
		a, b := base(), base()
		b.Score = 9.05
		assert.False(t, AnimeMetadataEqual(a, b))
	})

	t.Run("each metadata field breaks equality", func(t *testing.T) {
		mutators := map[string]func(*domain.Anime){
			"name":             func(x *domain.Anime) { x.Name = "x" },
			"name_en":          func(x *domain.Anime) { x.NameEN = "x" },
			"name_ru":          func(x *domain.Anime) { x.NameRU = "x" },
			"name_jp":          func(x *domain.Anime) { x.NameJP = "x" },
			"description":      func(x *domain.Anime) { x.Description = "x" },
			"year":             func(x *domain.Anime) { x.Year = 1999 },
			"season":           func(x *domain.Anime) { x.Season = "spring" },
			"status":           func(x *domain.Anime) { x.Status = domain.StatusOngoing },
			"kind":             func(x *domain.Anime) { x.Kind = "movie" },
			"rating":           func(x *domain.Anime) { x.Rating = "r" },
			"material_source":  func(x *domain.Anime) { x.MaterialSource = "novel" },
			"episodes_count":   func(x *domain.Anime) { x.EpisodesCount = 1 },
			"episodes_aired":   func(x *domain.Anime) { x.EpisodesAired = 1 },
			"episode_duration": func(x *domain.Anime) { x.EpisodeDuration = 1 },
			"poster_url":       func(x *domain.Anime) { x.PosterURL = "http://p/2.jpg" },
			"next_episode_at":  func(x *domain.Anime) { tt := next.Add(time.Hour); x.NextEpisodeAt = &tt },
			"next_episode_nil": func(x *domain.Anime) { x.NextEpisodeAt = nil },
			"next_episode_source": func(x *domain.Anime) {
				x.NextEpisodeSource = "anilist"
			},
			"aired_on": func(x *domain.Anime) { tt := aired.AddDate(0, 0, 1); x.AiredOn = &tt },
		}
		for name, mut := range mutators {
			a, b := base(), base()
			mut(b)
			assert.Falsef(t, AnimeMetadataEqual(a, b), "%s change should break equality", name)
		}
	})
}

func TestAnimeRepository_TouchUpdatedAt(t *testing.T) {
	db := setupAnimeUpdateTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()
	id := seedExistingAnime(t, db)

	var before domain.Anime
	require.NoError(t, db.Where("id = ?", id).First(&before).Error)

	// Advance updated_at without touching metadata.
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, r.TouchUpdatedAt(ctx, []string{id}))

	var after domain.Anime
	require.NoError(t, db.Where("id = ?", id).First(&after).Error)

	assert.True(t, after.UpdatedAt.After(before.UpdatedAt), "updated_at should advance")
	// Metadata and lazy/admin columns must be untouched.
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.Score, after.Score)
	assert.Equal(t, before.EpisodesAired, after.EpisodesAired)
	assert.Equal(t, before.SortPriority, after.SortPriority)
	assert.Equal(t, before.Hidden, after.Hidden)

	// Empty input is a no-op (no error, no accidental all-rows update).
	require.NoError(t, r.TouchUpdatedAt(ctx, nil))
}
