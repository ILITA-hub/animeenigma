package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/google/uuid"
)

type GenreRepository struct {
	db *database.DB
}

func NewGenreRepository(db *database.DB) *GenreRepository {
	return &GenreRepository{db: db}
}

func (r *GenreRepository) GetAll(ctx context.Context) ([]domain.Genre, error) {
	query := `SELECT id, name, name_ru FROM genres ORDER BY name`

	var genres []domain.Genre
	if err := r.db.SelectContext(ctx, &genres, query); err != nil {
		return nil, fmt.Errorf("get all genres: %w", err)
	}

	return genres, nil
}

func (r *GenreRepository) GetByID(ctx context.Context, id string) (*domain.Genre, error) {
	query := `SELECT id, name, name_ru FROM genres WHERE id = $1`

	var genre domain.Genre
	if err := r.db.GetContext(ctx, &genre, query, id); err != nil {
		return nil, fmt.Errorf("get genre by id: %w", err)
	}

	return &genre, nil
}

func (r *GenreRepository) GetByName(ctx context.Context, name string) (*domain.Genre, error) {
	query := `SELECT id, name, name_ru FROM genres WHERE name = $1`

	var genre domain.Genre
	if err := r.db.GetContext(ctx, &genre, query, name); err != nil {
		return nil, nil // Not found is OK
	}

	return &genre, nil
}

func (r *GenreRepository) Upsert(ctx context.Context, genre *domain.Genre) error {
	if genre.ID == "" {
		genre.ID = uuid.New().String()
	}

	query := `
		INSERT INTO genres (id, name, name_ru, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (name) DO UPDATE SET name_ru = $3, updated_at = $4
		RETURNING id
	`

	now := time.Now()
	return r.db.GetContext(ctx, &genre.ID, query, genre.ID, genre.Name, genre.NameRU, now)
}

func (r *GenreRepository) GetForAnime(ctx context.Context, animeID string) ([]domain.Genre, error) {
	query := `
		SELECT g.id, g.name, g.name_ru
		FROM genres g
		INNER JOIN anime_genres ag ON g.id = ag.genre_id
		WHERE ag.anime_id = $1
		ORDER BY g.name
	`

	var genres []domain.Genre
	if err := r.db.SelectContext(ctx, &genres, query, animeID); err != nil {
		return nil, fmt.Errorf("get genres for anime: %w", err)
	}

	return genres, nil
}

func (r *GenreRepository) SetAnimeGenres(ctx context.Context, animeID string, genreIDs []string) error {
	// Delete existing associations
	_, err := r.db.ExecContext(ctx, "DELETE FROM anime_genres WHERE anime_id = $1", animeID)
	if err != nil {
		return fmt.Errorf("delete anime genres: %w", err)
	}

	// Insert new associations
	if len(genreIDs) == 0 {
		return nil
	}

	query := `INSERT INTO anime_genres (anime_id, genre_id, created_at) VALUES ($1, $2, $3)`
	now := time.Now()

	for _, genreID := range genreIDs {
		if _, err := r.db.ExecContext(ctx, query, animeID, genreID, now); err != nil {
			return fmt.Errorf("insert anime genre: %w", err)
		}
	}

	return nil
}
