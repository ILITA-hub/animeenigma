package repo

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenreRepository struct {
	db *gorm.DB
}

func NewGenreRepository(db *gorm.DB) *GenreRepository {
	return &GenreRepository{db: db}
}

func (r *GenreRepository) GetAll(ctx context.Context) ([]domain.Genre, error) {
	var genres []domain.Genre
	if err := r.db.WithContext(ctx).Order("name").Find(&genres).Error; err != nil {
		return nil, fmt.Errorf("get all genres: %w", err)
	}
	return genres, nil
}

func (r *GenreRepository) GetByID(ctx context.Context, id string) (*domain.Genre, error) {
	var genre domain.Genre
	if err := r.db.WithContext(ctx).First(&genre, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("get genre by id: %w", err)
	}
	return &genre, nil
}

func (r *GenreRepository) GetByName(ctx context.Context, name string) (*domain.Genre, error) {
	var genre domain.Genre
	if err := r.db.WithContext(ctx).First(&genre, "name = ?", name).Error; err != nil {
		return nil, nil // Not found is OK
	}
	return &genre, nil
}

func (r *GenreRepository) Upsert(ctx context.Context, genre *domain.Genre) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "name_ru", "updated_at"}),
	}).Create(genre).Error
}

func (r *GenreRepository) GetForAnime(ctx context.Context, animeID string) ([]domain.Genre, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).Preload("Genres").First(&anime, "id = ?", animeID).Error; err != nil {
		return nil, fmt.Errorf("get genres for anime: %w", err)
	}
	return anime.Genres, nil
}

func (r *GenreRepository) SetAnimeGenres(ctx context.Context, animeID string, genreIDs []string) error {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).First(&anime, "id = ?", animeID).Error; err != nil {
		return fmt.Errorf("find anime: %w", err)
	}

	var genres []domain.Genre
	if len(genreIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", genreIDs).Find(&genres).Error; err != nil {
			return fmt.Errorf("find genres: %w", err)
		}
	}

	if err := r.db.WithContext(ctx).Model(&anime).Association("Genres").Replace(genres); err != nil {
		return fmt.Errorf("set anime genres: %w", err)
	}

	return nil
}
