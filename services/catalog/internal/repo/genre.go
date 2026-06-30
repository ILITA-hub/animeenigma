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

// GetForAnimes loads genres for multiple anime IDs in a single query.
// Returns a map from anime ID to its genres.
func (r *GenreRepository) GetForAnimes(ctx context.Context, animeIDs []string) (map[string][]domain.Genre, error) {
	if len(animeIDs) == 0 {
		return make(map[string][]domain.Genre), nil
	}

	// Query the join table + genres in one query
	type row struct {
		AnimeID string
		Genre   domain.Genre
	}

	var rows []struct {
		AnimeID string `gorm:"column:anime_id"`
		ID      string `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		NameRU  string `gorm:"column:name_ru"`
	}
	err := r.db.WithContext(ctx).
		Table("anime_genres").
		Select("anime_genres.anime_id, genres.id, genres.name, genres.name_ru").
		Joins("JOIN genres ON genres.id = anime_genres.genre_id").
		Where("anime_genres.anime_id IN ?", animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get genres for animes batch: %w", err)
	}

	result := make(map[string][]domain.Genre, len(animeIDs))
	for _, r := range rows {
		result[r.AnimeID] = append(result[r.AnimeID], domain.Genre{
			ID:     r.ID,
			Name:   r.Name,
			NameRU: r.NameRU,
		})
	}
	return result, nil
}

// animeGenreLink is the row shape of the GORM-managed `anime_genres` many2many
// join table. It exists only so ReplaceAnimeGenresBatch can address that table
// directly (there is no domain model for the join) for bulk delete/insert.
type animeGenreLink struct {
	AnimeID string `gorm:"column:anime_id"`
	GenreID string `gorm:"column:genre_id"`
}

func (animeGenreLink) TableName() string { return "anime_genres" }

// ReplaceAnimeGenresBatch replaces the genre links for many anime in a single
// transaction of two statements: one DELETE spanning every supplied anime ID,
// then one batched INSERT of all (anime_id, genre_id) pairs. It is the bulk
// equivalent of calling SetAnimeGenres per anime (which costs four statements
// each), used by BatchRefreshAnime over a whole chunk.
//
// animeGenres maps an anime ID to its genre IDs; a nil/empty slice clears that
// anime's links (full replace semantics). Anime IDs absent from the map are
// untouched. Referenced genres are assumed to already exist — callers upsert
// the genre rows first.
func (r *GenreRepository) ReplaceAnimeGenresBatch(ctx context.Context, animeGenres map[string][]string) error {
	if len(animeGenres) == 0 {
		return nil
	}

	animeIDs := make([]string, 0, len(animeGenres))
	links := make([]animeGenreLink, 0, len(animeGenres)*4) // ~4 genres/anime
	for animeID, genreIDs := range animeGenres {
		animeIDs = append(animeIDs, animeID)
		for _, gid := range genreIDs {
			if gid == "" {
				continue
			}
			// Duplicate (anime_id, genre_id) pairs are absorbed by the
			// OnConflict{DoNothing} on the INSERT below.
			links = append(links, animeGenreLink{AnimeID: animeID, GenreID: gid})
		}
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Clear existing links for all these anime in one statement.
		if err := tx.Where("anime_id IN ?", animeIDs).Delete(&animeGenreLink{}).Error; err != nil {
			return fmt.Errorf("clear anime genres: %w", err)
		}
		if len(links) == 0 {
			return nil
		}
		// Re-insert every link in one batched statement. DoNothing guards the
		// composite (anime_id, genre_id) primary key against a duplicate pair.
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(links, 1000).Error; err != nil {
			return fmt.Errorf("insert anime genres: %w", err)
		}
		return nil
	})
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
