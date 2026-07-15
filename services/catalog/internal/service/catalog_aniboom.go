package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// GetAniboomTranslations gets available translations from Aniboom for an anime
func (s *CatalogService) GetAniboomTranslations(ctx context.Context, animeID string) ([]domain.AniboomTranslation, error) {
	// Get anime to get names for search
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("aniboom:translations:%s", animeID)
	var cached []domain.AniboomTranslation
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Search on Aniboom by anime name
	searchResult, err := s.aniboomClient.SearchByShikimoriName(anime.NameRU, anime.NameJP, anime.Name)
	if err != nil {
		s.log.Warnw("failed to find anime on aniboom", "anime_id", animeID, "error", err)
		return nil, errors.NotFound("anime not found on aniboom")
	}

	s.log.Infow("found anime on aniboom",
		"anime_id", animeID,
		"animego_id", searchResult.AnimegoID,
		"title", searchResult.Title)

	// Get translations
	translations, err := s.aniboomClient.GetTranslations(searchResult.AnimegoID)
	if err != nil {
		s.log.Warnw("failed to get aniboom translations", "error", err)
		return nil, err
	}

	result := make([]domain.AniboomTranslation, len(translations))
	for i, t := range translations {
		result[i] = domain.AniboomTranslation{
			Name:          t.Name,
			TranslationID: t.TranslationID,
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetAniboomVideoSource gets video source URL from Aniboom for a specific episode
func (s *CatalogService) GetAniboomVideoSource(ctx context.Context, animeID string, episode int, translationID string) (*domain.AniboomVideoSource, error) {
	// Get anime to get names for search
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("aniboom:video:%s:%d:%s", animeID, episode, translationID)
	var cached domain.AniboomVideoSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	// Search on Aniboom by anime name
	searchResult, err := s.aniboomClient.SearchByShikimoriName(anime.NameRU, anime.NameJP, anime.Name)
	if err != nil {
		return nil, errors.NotFound("anime not found on aniboom")
	}

	// Get video source
	source, err := s.aniboomClient.GetVideoSource(searchResult.AnimegoID, episode, translationID)
	if err != nil {
		s.log.Warnw("failed to get aniboom video source",
			"anime_id", animeID,
			"episode", episode,
			"translation_id", translationID,
			"error", err)
		return nil, err
	}

	result := &domain.AniboomVideoSource{
		URL:           source.URL,
		Type:          source.Type,
		Episode:       episode,
		TranslationID: translationID,
	}

	// Cache for 1 hour (video URLs expire)
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}
