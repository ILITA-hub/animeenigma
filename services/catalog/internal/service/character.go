package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
)

// characterShikimori is the slice of the Shikimori client this service needs.
type characterShikimori interface {
	GetAnimeCharacters(ctx context.Context, shikimoriID string) ([]shikimori.CharacterRoleResult, error)
	GetCharacterByID(ctx context.Context, shikimoriID string) (*domain.Character, error)
}

// characterRepo is the slice of CharacterRepository this service needs.
type characterRepo interface {
	UpsertCharacter(ctx context.Context, ch *domain.Character) (*domain.Character, error)
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Character, error)
	ReplaceAnimeCharacters(ctx context.Context, animeID string, rows []domain.AnimeCharacter, chars []domain.Character) error
	GetByAnimeID(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error)
}

// animeShikimoriIDLookup resolves a catalog anime UUID to its Shikimori id.
type animeShikimoriIDLookup interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
}

// CharacterService orchestrates Shikimori fetch → Postgres upsert → Redis cache.
type CharacterService struct {
	animeRepo animeShikimoriIDLookup
	chars     characterRepo
	shikimori characterShikimori
	cache     *cache.RedisCache
	log       *logger.Logger
}

func NewCharacterService(
	animeRepo animeShikimoriIDLookup,
	chars characterRepo,
	shiki characterShikimori,
	c *cache.RedisCache,
	log *logger.Logger,
) *CharacterService {
	return &CharacterService{animeRepo: animeRepo, chars: chars, shikimori: shiki, cache: c, log: log}
}

// GetAnimeCharacters returns an anime's characters. Flow: Redis → (miss)
// fetch Shikimori → upsert Postgres → cache → return; on Shikimori failure,
// serve last-known-good from Postgres.
func (s *CharacterService) GetAnimeCharacters(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error) {
	cacheKey := cache.KeyAnimeCharacters(animeID)
	var cached []domain.AnimeCharacterView
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	roles, ferr := s.shikimori.GetAnimeCharacters(ctx, anime.ShikimoriID)
	if ferr != nil {
		s.log.Warnw("shikimori characters fetch failed, serving from db", "anime_id", animeID, "error", ferr)
		return s.chars.GetByAnimeID(ctx, animeID)
	}

	rows := make([]domain.AnimeCharacter, 0, len(roles))
	for i := range roles {
		stored, uerr := s.chars.UpsertCharacter(ctx, &roles[i].Character)
		if uerr != nil {
			return nil, uerr
		}
		rows = append(rows, domain.AnimeCharacter{
			AnimeID:     animeID,
			CharacterID: stored.ID,
			Role:        roles[i].Role,
			Position:    i,
		})
	}
	if err := s.chars.ReplaceAnimeCharacters(ctx, animeID, rows, nil); err != nil {
		return nil, err
	}

	views, err := s.chars.GetByAnimeID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, views, cache.TTLAnimeDetails)
	return views, nil
}

// GetCharacter returns a single character by Shikimori id. Flow: Redis →
// (miss) Shikimori → upsert Postgres → cache; on Shikimori failure, Postgres.
func (s *CharacterService) GetCharacter(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	cacheKey := cache.KeyCharacter(shikimoriID)
	var cached domain.Character
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	ch, ferr := s.shikimori.GetCharacterByID(ctx, shikimoriID)
	if ferr != nil {
		s.log.Warnw("shikimori character fetch failed, serving from db", "shikimori_id", shikimoriID, "error", ferr)
		return s.chars.GetByShikimoriID(ctx, shikimoriID)
	}

	stored, err := s.chars.UpsertCharacter(ctx, ch)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, stored, cache.TTLAnimeDetails)
	return stored, nil
}
