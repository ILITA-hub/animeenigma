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
			episodes_count, episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		anime.ID, anime.Name, anime.NameRU, anime.NameJP, anime.Description,
		anime.Year, anime.Season, anime.Status, anime.EpisodesCount, anime.EpisodesAired,
		anime.EpisodeDuration, anime.Score, anime.PosterURL,
		anime.ShikimoriID, anime.MALID, anime.AniListID, anime.HasVideo,
		anime.NextEpisodeAt, anime.AiredOn,
		anime.CreatedAt, anime.UpdatedAt)

	if err != nil {
		return fmt.Errorf("create anime: %w", err)
	}

	return nil
}

func (r *AnimeRepository) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
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
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
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

func (r *AnimeRepository) GetByMALID(ctx context.Context, malID string) (*domain.Anime, error) {
	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
		FROM anime
		WHERE mal_id = $1
	`

	var anime domain.Anime
	err := r.db.GetContext(ctx, &anime, query, malID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error for this use case
		}
		return nil, fmt.Errorf("get anime by mal id: %w", err)
	}

	return &anime, nil
}

func (r *AnimeRepository) Update(ctx context.Context, anime *domain.Anime) error {
	anime.UpdatedAt = time.Now()

	query := `
		UPDATE anime SET
			name = $1, name_ru = $2, name_jp = $3, description = $4,
			year = $5, season = $6, status = $7, episodes_count = $8,
			episodes_aired = $9, episode_duration = $10, score = $11, poster_url = $12,
			shikimori_id = $13, mal_id = $14, anilist_id = $15,
			has_video = $16, next_episode_at = $17, aired_on = $18, updated_at = $19
		WHERE id = $20
	`

	result, err := r.db.ExecContext(ctx, query,
		anime.Name, anime.NameRU, anime.NameJP, anime.Description,
		anime.Year, anime.Season, anime.Status, anime.EpisodesCount, anime.EpisodesAired,
		anime.EpisodeDuration, anime.Score, anime.PosterURL,
		anime.ShikimoriID, anime.MALID, anime.AniListID, anime.HasVideo,
		anime.NextEpisodeAt, anime.AiredOn, anime.UpdatedAt, anime.ID)

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
		// Map frontend sort values to database columns
		column := mapSortColumn(filters.Sort)
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
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
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

// GetSchedule returns ongoing anime with next episode dates, grouped by day of week
func (r *AnimeRepository) GetSchedule(ctx context.Context) ([]*domain.Anime, error) {
	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
		FROM anime
		WHERE status = 'ongoing' AND next_episode_at IS NOT NULL AND next_episode_at > NOW()
		ORDER BY next_episode_at ASC
	`

	var animes []*domain.Anime
	if err := r.db.SelectContext(ctx, &animes, query); err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}

	return animes, nil
}

// GetOngoingAnime returns all ongoing anime
func (r *AnimeRepository) GetOngoingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	// Count total
	var total int64
	countQuery := `SELECT COUNT(*) FROM anime WHERE status = 'ongoing'`
	if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, fmt.Errorf("count ongoing anime: %w", err)
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, name, name_ru, name_jp, description, year, season, status,
			episodes_count, COALESCE(episodes_aired, 0) as episodes_aired, episode_duration, score, poster_url,
			shikimori_id, mal_id, anilist_id, has_video, next_episode_at, aired_on, created_at, updated_at
		FROM anime
		WHERE status = 'ongoing'
		ORDER BY COALESCE(next_episode_at, '9999-12-31') ASC, score DESC
		LIMIT $1 OFFSET $2
	`

	var animes []*domain.Anime
	if err := r.db.SelectContext(ctx, &animes, query, pageSize, offset); err != nil {
		return nil, 0, fmt.Errorf("get ongoing anime: %w", err)
	}

	return animes, total, nil
}

// mapSortColumn maps frontend sort values to database column names
func mapSortColumn(sort string) string {
	switch sort {
	case "popularity":
		return "score" // Use score as proxy for popularity
	case "rating":
		return "score"
	case "year":
		return "year"
	case "title":
		return "name"
	case "score", "name", "created_at", "updated_at":
		return sort // Already valid column names
	default:
		return "score" // Default fallback
	}
}

// GetPinnedTranslations returns all pinned translations for an anime
func (r *AnimeRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	query := `
		SELECT anime_id, translation_id, translation_title, translation_type, pinned_at
		FROM pinned_translations
		WHERE anime_id = $1
		ORDER BY pinned_at ASC
	`

	var pinned []domain.PinnedTranslation
	err := r.db.SelectContext(ctx, &pinned, query, animeID)
	if err != nil {
		return nil, fmt.Errorf("get pinned translations: %w", err)
	}

	return pinned, nil
}

// PinTranslation pins a translation for an anime
func (r *AnimeRepository) PinTranslation(ctx context.Context, pin *domain.PinnedTranslation) error {
	query := `
		INSERT INTO pinned_translations (anime_id, translation_id, translation_title, translation_type, pinned_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (anime_id, translation_id) DO UPDATE SET
			translation_title = EXCLUDED.translation_title,
			translation_type = EXCLUDED.translation_type
	`

	_, err := r.db.ExecContext(ctx, query,
		pin.AnimeID, pin.TranslationID, pin.TranslationTitle, pin.TranslationType, time.Now())
	if err != nil {
		return fmt.Errorf("pin translation: %w", err)
	}

	return nil
}

// UnpinTranslation removes a pinned translation for an anime
func (r *AnimeRepository) UnpinTranslation(ctx context.Context, animeID string, translationID int) error {
	query := `DELETE FROM pinned_translations WHERE anime_id = $1 AND translation_id = $2`

	result, err := r.db.ExecContext(ctx, query, animeID, translationID)
	if err != nil {
		return fmt.Errorf("unpin translation: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.NotFound("pinned translation not found")
	}

	return nil
}
