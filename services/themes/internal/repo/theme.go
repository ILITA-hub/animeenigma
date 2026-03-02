package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ThemeRepository struct {
	db *gorm.DB
}

func NewThemeRepository(db *gorm.DB) *ThemeRepository {
	return &ThemeRepository{db: db}
}

// Upsert creates or updates a theme by external_id.
func (r *ThemeRepository) Upsert(ctx context.Context, theme *domain.AnimeTheme) error {
	now := time.Now()
	theme.UpdatedAt = now
	if theme.CreatedAt.IsZero() {
		theme.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "external_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"anime_name", "anime_slug", "poster_url", "theme_type",
			"sequence", "slug", "song_title", "artist_name",
			"video_basename", "video_resolution", "audio_basename",
			"mal_id", "year", "season", "updated_at",
		}),
	}).Create(theme).Error
}

// List returns themes with optional filters, including avg score and vote count.
// LEFT JOINs the animes table to link themes with local catalog entries by MAL ID.
func (r *ThemeRepository) List(ctx context.Context, params domain.ThemeListParams) ([]domain.AnimeTheme, error) {
	query := r.db.WithContext(ctx).
		Table("anime_themes").
		Select(`anime_themes.*,
			COALESCE(AVG(theme_ratings.score), 0) as avg_score,
			COUNT(theme_ratings.id) as vote_count,
			animes.id as anime_id,
			COALESCE(NULLIF(animes.name_ru, ''), NULLIF(animes.name, ''), anime_themes.anime_name) as anime_name`).
		Joins("LEFT JOIN theme_ratings ON theme_ratings.theme_id = anime_themes.id").
		Joins("LEFT JOIN animes ON anime_themes.mal_id > 0 AND anime_themes.mal_id::text = animes.shikimori_id AND animes.deleted_at IS NULL").
		Where("anime_themes.deleted_at IS NULL").
		Group("anime_themes.id, animes.id, animes.name, animes.name_ru")

	if params.Year > 0 {
		query = query.Where("anime_themes.year = ?", params.Year)
	}
	if params.Season != "" {
		query = query.Where("anime_themes.season = ?", params.Season)
	}
	if params.Type != "" {
		query = query.Where("anime_themes.theme_type = ?", params.Type)
	}

	switch params.Sort {
	case "name":
		query = query.Order("anime_themes.anime_name ASC, anime_themes.slug ASC")
	case "newest":
		query = query.Order("anime_themes.created_at DESC")
	default: // "rating"
		query = query.Order("avg_score DESC, vote_count DESC, anime_themes.anime_name ASC")
	}

	var themes []domain.AnimeTheme
	if err := query.Scan(&themes).Error; err != nil {
		return nil, err
	}
	return themes, nil
}

// GetByID returns a single theme with avg score and vote count.
func (r *ThemeRepository) GetByID(ctx context.Context, id string) (*domain.AnimeTheme, error) {
	var theme domain.AnimeTheme
	err := r.db.WithContext(ctx).
		Table("anime_themes").
		Select(`anime_themes.*,
			COALESCE(AVG(theme_ratings.score), 0) as avg_score,
			COUNT(theme_ratings.id) as vote_count,
			animes.id as anime_id,
			COALESCE(NULLIF(animes.name_ru, ''), NULLIF(animes.name, ''), anime_themes.anime_name) as anime_name`).
		Joins("LEFT JOIN theme_ratings ON theme_ratings.theme_id = anime_themes.id").
		Joins("LEFT JOIN animes ON anime_themes.mal_id > 0 AND anime_themes.mal_id::text = animes.shikimori_id AND animes.deleted_at IS NULL").
		Where("anime_themes.id = ? AND anime_themes.deleted_at IS NULL", id).
		Group("anime_themes.id, animes.id, animes.name, animes.name_ru").
		Scan(&theme).Error

	if err != nil {
		return nil, err
	}
	return &theme, nil
}
