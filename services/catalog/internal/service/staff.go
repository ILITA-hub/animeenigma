package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// staffShikimori is the slice of the Shikimori client this service needs.
type staffShikimori interface {
	GetAnimeStaff(ctx context.Context, shikimoriID string) ([]domain.AnimePersonRole, error)
}

// staffRepo is the slice of PersonRoleRepository this service needs.
type staffRepo interface {
	ReplaceAnimeStaff(ctx context.Context, animeID string, rows []domain.AnimePersonRole) error
	GetStaffByAnimeID(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error)
}

// StaffService orchestrates Shikimori fetch → Postgres replace → Redis cache
// for an anime's crew. Same resilience contract as CharacterService: on a
// Shikimori failure it serves the last-known-good rows from Postgres.
type StaffService struct {
	animeRepo animeShikimoriIDLookup // defined in character.go (same package)
	staff     staffRepo
	shikimori staffShikimori
	cache     *cache.RedisCache
	log       *logger.Logger
}

func NewStaffService(
	animeRepo animeShikimoriIDLookup,
	staff staffRepo,
	shiki staffShikimori,
	c *cache.RedisCache,
	log *logger.Logger,
) *StaffService {
	return &StaffService{animeRepo: animeRepo, staff: staff, shikimori: shiki, cache: c, log: log}
}

// GetAnimeStaff returns an anime's crew. Flow: Redis → (miss) resolve
// shikimori id → fetch → replace Postgres → cache → return; on Shikimori
// failure, serve last-known-good from Postgres.
func (s *StaffService) GetAnimeStaff(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error) {
	cacheKey := cache.KeyAnimeStaff(animeID)
	var cached []domain.AnimePersonRole
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	rows, ferr := s.shikimori.GetAnimeStaff(ctx, anime.ShikimoriID)
	if ferr != nil {
		s.log.Warnw("shikimori staff fetch failed, serving from db", "anime_id", animeID, "error", ferr)
		return s.staff.GetStaffByAnimeID(ctx, animeID)
	}

	for i := range rows {
		rows[i].AnimeID = animeID
	}
	if err := s.staff.ReplaceAnimeStaff(ctx, animeID, rows); err != nil {
		return nil, err
	}

	stored, err := s.staff.GetStaffByAnimeID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, stored, cache.TTLAnimeDetails)
	return stored, nil
}
