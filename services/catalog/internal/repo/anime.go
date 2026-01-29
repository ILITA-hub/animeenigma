package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/google/uuid"
)

type AnimeRepository struct {
	db *database.DB
}

func NewAnimeRepository(db *database.DB) *AnimeRepository {
	return &AnimeRepository{db: db}
}

func (r *AnimeRepository) Create(ctx context.Context, anime *domain.Anime) error {
	anime.ID = uuid.New().String()
	anime.CreatedAt = time.Now()
	anime.UpdatedAt = time.Now()

	query := `
		INSERT INTO anime (
			id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		anime.ID, anime.Name, anime.NameRU, anime.NameJP, anime.Description,
		anime.Year, anime.Season, anime.Status, anime.EpisodesCount,
		anime.EpisodeDuration, anime.Score, anime.PosterURL,
		anime.ShikimoriID, anime.MALID, anime.AniListID, anime.HasVideo,
		anime.CreatedAt, anime.UpdatedAt)

	if err != nil {
		return fmt.Errorf("create anime: %w", err)
	}

	return nil
}

func (r *AnimeRepository) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, created_at, updated_at
		FROM anime
		WHERE id = $1
	`

	var anime domain.Anime
	err := r.db.GetContext(ctx, &anime, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFound("anime")
		}
		return nil, fmt.Errorf("get anime by id: %w", err)
	}

	return &anime, nil
}

func (r *AnimeRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error) {
	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, created_at, updated_at
		FROM anime
		WHERE shikimori_id = $1
	`

	var anime domain.Anime
	err := r.db.GetContext(ctx, &anime, query, shikimoriID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error for this use case
		}
		return nil, fmt.Errorf("get anime by shikimori id: %w", err)
	}

	return &anime, nil
}

func (r *AnimeRepository) Update(ctx context.Context, anime *domain.Anime) error {
	anime.UpdatedAt = time.Now()

	query := `
		UPDATE anime SET
			name = $1, name_ru = $2, name_jp = $3, description = $4,
			year = $5, season = $6, status = $7, episodes_count = $8,
			episode_duration = $9, score = $10, poster_url = $11,
			shikimori_id = $12, mal_id = $13, anilist_id = $14,
			has_video = $15, updated_at = $16
		WHERE id = $17
	`

	result, err := r.db.ExecContext(ctx, query,
		anime.Name, anime.NameRU, anime.NameJP, anime.Description,
		anime.Year, anime.Season, anime.Status, anime.EpisodesCount,
		anime.EpisodeDuration, anime.Score, anime.PosterURL,
		anime.ShikimoriID, anime.MALID, anime.AniListID, anime.HasVideo,
		anime.UpdatedAt, anime.ID)

	if err != nil {
		return fmt.Errorf("update anime: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.NotFound("anime")
	}

	return nil
}

func (r *AnimeRepository) Search(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Build WHERE conditions
	if filters.Query != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(name ILIKE $%d OR name_ru ILIKE $%d OR name_jp ILIKE $%d)",
			argIndex, argIndex, argIndex))
		args = append(args, "%"+filters.Query+"%")
		argIndex++
	}

	if filters.Year != nil {
		conditions = append(conditions, fmt.Sprintf("year = $%d", argIndex))
		args = append(args, *filters.Year)
		argIndex++
	}

	if filters.YearFrom != nil {
		conditions = append(conditions, fmt.Sprintf("year >= $%d", argIndex))
		args = append(args, *filters.YearFrom)
		argIndex++
	}

	if filters.YearTo != nil {
		conditions = append(conditions, fmt.Sprintf("year <= $%d", argIndex))
		args = append(args, *filters.YearTo)
		argIndex++
	}

	if filters.Season != "" {
		conditions = append(conditions, fmt.Sprintf("season = $%d", argIndex))
		args = append(args, filters.Season)
		argIndex++
	}

	if filters.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, filters.Status)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM anime %s", whereClause)
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count anime: %w", err)
	}

	// Build ORDER BY
	orderBy := "score DESC"
	if filters.Sort != "" {
		column := filters.Sort
		order := "DESC"
		if filters.Order == "asc" {
			order = "ASC"
		}
		orderBy = fmt.Sprintf("%s %s", column, order)
	}

	// Pagination
	offset := (filters.Page - 1) * filters.PageSize
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, created_at, updated_at
		FROM anime
		%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderBy, argIndex, argIndex+1)

	args = append(args, filters.PageSize, offset)

	var animes []*domain.Anime
	if err := r.db.SelectContext(ctx, &animes, query, args...); err != nil {
		return nil, 0, fmt.Errorf("search anime: %w", err)
	}

	return animes, total, nil
}

func (r *AnimeRepository) GetBySeason(ctx context.Context, year int, season string, page, pageSize int) ([]*domain.Anime, int64, error) {
	filters := domain.SearchFilters{
		Year:     &year,
		Season:   season,
		Page:     page,
		PageSize: pageSize,
		Sort:     "score",
		Order:    "desc",
	}
	return r.Search(ctx, filters)
}

func (r *AnimeRepository) SetHasVideo(ctx context.Context, animeID string, hasVideo bool) error {
	query := `UPDATE anime SET has_video = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, hasVideo, time.Now(), animeID)
	return err
}
