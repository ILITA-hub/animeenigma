package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// GetVideosForAnime gets all videos for an anime
func (s *CatalogService) GetVideosForAnime(ctx context.Context, animeID string, videoType domain.VideoType) ([]*domain.Video, error) {
	return s.videoRepo.GetForAnime(ctx, animeID, videoType)
}

// GetVideosForEpisode gets video sources for a specific episode
func (s *CatalogService) GetVideosForEpisode(ctx context.Context, animeID string, episodeNumber int) ([]*domain.Video, error) {
	return s.videoRepo.GetForEpisode(ctx, animeID, episodeNumber)
}

// GetRandomVideos gets random videos for the game
func (s *CatalogService) GetRandomVideos(ctx context.Context, videoType domain.VideoType, count int, excludeIDs []string) ([]*domain.Video, error) {
	return s.videoRepo.GetRandomVideos(ctx, videoType, count, excludeIDs)
}

// CreateAnime creates an anime manually (admin)
func (s *CatalogService) CreateAnime(ctx context.Context, req *domain.CreateAnimeRequest) (*domain.Anime, error) {
	// If Shikimori ID provided, fetch and merge data
	if req.ShikimoriID != "" {
		shikimoriAnime, err := s.shikimoriClient.GetAnimeByID(ctx, req.ShikimoriID)
		if err != nil {
			return nil, fmt.Errorf("fetch from shikimori: %w", err)
		}

		// Override with provided values
		if req.Name != "" {
			shikimoriAnime.Name = req.Name
		}
		if req.NameRU != "" {
			shikimoriAnime.NameRU = req.NameRU
		}
		if req.NameJP != "" {
			shikimoriAnime.NameJP = req.NameJP
		}
		if req.Description != "" {
			shikimoriAnime.Description = req.Description
		}
		if req.MALID != "" {
			shikimoriAnime.MALID = req.MALID
		}

		if err := s.upsertAnimeFromExternal(ctx, shikimoriAnime); err != nil {
			return nil, err
		}

		return shikimoriAnime, nil
	}

	// Create manually
	anime := &domain.Anime{
		Name:          req.Name,
		NameRU:        req.NameRU,
		NameJP:        req.NameJP,
		Description:   req.Description,
		Year:          req.Year,
		Season:        req.Season,
		Status:        domain.AnimeStatus(req.Status),
		EpisodesCount: req.EpisodesCount,
		PosterURL:     req.PosterURL,
		MALID:         req.MALID,
	}

	if err := s.animeRepo.Create(ctx, anime); err != nil {
		return nil, err
	}

	// Set genres
	if len(req.GenreIDs) > 0 {
		if err := s.genreRepo.SetAnimeGenres(ctx, anime.ID, req.GenreIDs); err != nil {
			s.log.Warnw("failed to set genres", "error", err)
		}
	}

	return anime, nil
}

// AddVideoSource adds a video source to an anime (admin)
func (s *CatalogService) AddVideoSource(ctx context.Context, animeID string, req *domain.AddVideoRequest) (*domain.Video, error) {
	// Verify anime exists
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	video := &domain.Video{
		AnimeID:       animeID,
		Type:          domain.VideoTypeEpisode,
		EpisodeNumber: req.EpisodeNumber,
		SourceType:    req.SourceType,
		Quality:       req.Quality,
		Language:      req.Language,
	}

	if req.SourceType == domain.SourceTypeExternal {
		if req.ExternalURL == "" {
			return nil, errors.InvalidInput("external_url is required for external source type")
		}
		video.SourceURL = req.ExternalURL
	}

	if err := s.videoRepo.Create(ctx, video); err != nil {
		return nil, err
	}

	// Update anime has_video flag
	if !anime.HasVideo {
		_ = s.animeRepo.SetHasVideo(ctx, animeID, true)
	}

	// Invalidate cache
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	return video, nil
}

// DeleteVideo deletes a video source (admin)
func (s *CatalogService) DeleteVideo(ctx context.Context, videoID string) error {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return err
	}

	if err := s.videoRepo.Delete(ctx, videoID); err != nil {
		return err
	}

	// Check if anime still has videos
	hasVideos, err := s.videoRepo.HasVideosForAnime(ctx, video.AnimeID)
	if err == nil && !hasVideos {
		_ = s.animeRepo.SetHasVideo(ctx, video.AnimeID, false)
	}

	// Invalidate cache
	_ = s.cache.Delete(ctx, cache.KeyAnime(video.AnimeID))

	return nil
}

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

// GetPinnedTranslations returns all pinned translations for an anime
func (s *CatalogService) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	return s.animeRepo.GetPinnedTranslations(ctx, animeID)
}

// PinTranslation pins a translation for an anime (admin only)
func (s *CatalogService) PinTranslation(ctx context.Context, animeID string, req domain.PinTranslationRequest) error {
	pin := &domain.PinnedTranslation{
		AnimeID:          animeID,
		TranslationID:    req.TranslationID,
		TranslationTitle: req.TranslationTitle,
		TranslationType:  req.TranslationType,
	}
	return s.animeRepo.PinTranslation(ctx, pin)
}

// UnpinTranslation removes a pinned translation for an anime (admin only)
func (s *CatalogService) UnpinTranslation(ctx context.Context, animeID string, translationID int) error {
	return s.animeRepo.UnpinTranslation(ctx, animeID, translationID)
}

// SetAnimeHidden sets the hidden status of an anime (admin only)
func (s *CatalogService) SetAnimeHidden(ctx context.Context, animeID string, hidden bool) error {
	return s.animeRepo.SetHidden(ctx, animeID, hidden)
}

// GetHiddenAnime returns all hidden anime (admin only)
func (s *CatalogService) GetHiddenAnime(ctx context.Context) ([]*domain.Anime, error) {
	animes, err := s.animeRepo.GetHiddenAnime(ctx)
	if err != nil {
		return nil, err
	}

	s.enrichAll(ctx, animes)

	return animes, nil
}

// LinkMALID links a MAL ID to an existing anime (admin only)
func (s *CatalogService) LinkMALID(ctx context.Context, animeID string, malID string) error {
	// Verify anime exists
	if _, err := s.animeRepo.GetByID(ctx, animeID); err != nil {
		return err
	}

	if err := s.animeRepo.UpdateMALID(ctx, animeID, malID); err != nil {
		return err
	}

	// Invalidate cache
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	return nil
}

// UpdateShikimoriID updates the Shikimori ID for an anime (admin only)
func (s *CatalogService) UpdateShikimoriID(ctx context.Context, animeID string, shikimoriID string) error {
	// Invalidate cache for kodik translations
	_ = s.cache.Delete(ctx, fmt.Sprintf("kodik:translations:%s", animeID))
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	return s.animeRepo.UpdateShikimoriID(ctx, animeID, shikimoriID)
}
