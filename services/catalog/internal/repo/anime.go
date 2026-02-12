package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

type AnimeRepository struct {
	db *gorm.DB
}

func NewAnimeRepository(db *gorm.DB) *AnimeRepository {
	return &AnimeRepository{db: db}
}

func (r *AnimeRepository) Create(ctx context.Context, anime *domain.Anime) error {
	if err := r.db.WithContext(ctx).Create(anime).Error; err != nil {
		return fmt.Errorf("create anime: %w", err)
	}
	return nil
}

func (r *AnimeRepository) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).Preload("Genres").First(&anime, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("anime")
		}
		return nil, fmt.Errorf("get anime by id: %w", err)
	}
	return &anime, nil
}

func (r *AnimeRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).First(&anime, "shikimori_id = ?", shikimoriID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get anime by shikimori id: %w", err)
	}
	return &anime, nil
}

func (r *AnimeRepository) GetByMALID(ctx context.Context, malID string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).First(&anime, "mal_id = ?", malID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get anime by mal id: %w", err)
	}
	return &anime, nil
}

func (r *AnimeRepository) Update(ctx context.Context, anime *domain.Anime) error {
	result := r.db.WithContext(ctx).Save(anime)
	if result.Error != nil {
		return fmt.Errorf("update anime: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) Search(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	query := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("hidden = ? OR hidden IS NULL", false)

	if filters.Query != "" {
		query = query.Where("name ILIKE ? OR name_ru ILIKE ? OR name_jp ILIKE ?",
			"%"+filters.Query+"%", "%"+filters.Query+"%", "%"+filters.Query+"%")
	}
	if filters.Year != nil {
		query = query.Where("year = ?", *filters.Year)
	}
	if filters.YearFrom != nil {
		query = query.Where("year >= ?", *filters.YearFrom)
	}
	if filters.YearTo != nil {
		query = query.Where("year <= ?", *filters.YearTo)
	}
	if filters.Season != "" {
		query = query.Where("season = ?", filters.Season)
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count anime: %w", err)
	}

	orderBy := "score DESC"
	if filters.Sort != "" {
		column := mapSortColumn(filters.Sort)
		order := "DESC"
		if filters.Order == "asc" {
			order = "ASC"
		}
		orderBy = fmt.Sprintf("%s %s", column, order)
	}

	offset := (filters.Page - 1) * filters.PageSize
	if offset < 0 {
		offset = 0
	}

	var animes []*domain.Anime
	if err := query.Order(orderBy).Limit(filters.PageSize).Offset(offset).Find(&animes).Error; err != nil {
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
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_video", hasVideo).Error
}

func (r *AnimeRepository) SetHidden(ctx context.Context, animeID string, hidden bool) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("hidden", hidden)
	if result.Error != nil {
		return fmt.Errorf("set hidden: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) GetHiddenAnime(ctx context.Context) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	if err := r.db.WithContext(ctx).Where("hidden = ?", true).Order("updated_at DESC").Find(&animes).Error; err != nil {
		return nil, fmt.Errorf("get hidden anime: %w", err)
	}
	return animes, nil
}

func (r *AnimeRepository) UpdateMALID(ctx context.Context, animeID string, malID string) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("mal_id", malID)
	if result.Error != nil {
		return fmt.Errorf("update mal_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) UpdateShikimoriID(ctx context.Context, animeID string, shikimoriID string) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("shikimori_id", shikimoriID)
	if result.Error != nil {
		return fmt.Errorf("update shikimori_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) GetSchedule(ctx context.Context) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_episode_at IS NOT NULL AND next_episode_at > NOW() AND (hidden = ? OR hidden IS NULL)",
			"ongoing", false).
		Order("next_episode_at ASC").
		Find(&animes).Error
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return animes, nil
}

func (r *AnimeRepository) GetOngoingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&domain.Anime{}).
		Where("status = ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count ongoing anime: %w", err)
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	var animes []*domain.Anime
	err := query.Order("COALESCE(next_episode_at, '9999-12-31') ASC, score DESC").
		Limit(pageSize).Offset(offset).Find(&animes).Error
	if err != nil {
		return nil, 0, fmt.Errorf("get ongoing anime: %w", err)
	}

	return animes, total, nil
}

func mapSortColumn(sort string) string {
	switch sort {
	case "popularity", "rating":
		return "score"
	case "year", "score", "name", "created_at", "updated_at":
		return sort
	case "title":
		return "name"
	default:
		return "score"
	}
}

func (r *AnimeRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	var pinned []domain.PinnedTranslation
	err := r.db.WithContext(ctx).Where("anime_id = ?", animeID).Order("pinned_at ASC").Find(&pinned).Error
	if err != nil {
		return nil, fmt.Errorf("get pinned translations: %w", err)
	}
	return pinned, nil
}

func (r *AnimeRepository) PinTranslation(ctx context.Context, pin *domain.PinnedTranslation) error {
	pin.PinnedAt = time.Now()
	result := r.db.WithContext(ctx).Save(pin)
	if result.Error != nil {
		return fmt.Errorf("pin translation: %w", result.Error)
	}
	return nil
}

func (r *AnimeRepository) UnpinTranslation(ctx context.Context, animeID string, translationID int) error {
	result := r.db.WithContext(ctx).Where("anime_id = ? AND translation_id = ?", animeID, translationID).
		Delete(&domain.PinnedTranslation{})
	if result.Error != nil {
		return fmt.Errorf("unpin translation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("pinned translation not found")
	}
	return nil
}
