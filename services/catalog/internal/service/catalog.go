package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type CatalogService struct {
	animeRepo      *repo.AnimeRepository
	genreRepo      *repo.GenreRepository
	videoRepo      *repo.VideoRepository
	shikimoriClient *shikimori.Client
	cache          *cache.RedisCache
	log            *logger.Logger
}

func NewCatalogService(
	animeRepo *repo.AnimeRepository,
	genreRepo *repo.GenreRepository,
	videoRepo *repo.VideoRepository,
	shikimoriClient *shikimori.Client,
	cache *cache.RedisCache,
	log *logger.Logger,
) *CatalogService {
	return &CatalogService{
		animeRepo:       animeRepo,
		genreRepo:       genreRepo,
		videoRepo:       videoRepo,
		shikimoriClient: shikimoriClient,
		cache:           cache,
		log:             log,
	}
}

// SearchAnime searches for anime, fetching from Shikimori if not found locally
func (s *CatalogService) SearchAnime(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	// First, try to search locally
	animes, total, err := s.animeRepo.Search(ctx, filters)
	if err != nil {
		return nil, 0, err
	}

	// If we have local results, enrich and return them
	if len(animes) > 0 {
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return animes, total, nil
	}

	// No local results - fetch from Shikimori
	if filters.Query != "" {
		s.log.Infow("no local results, fetching from Shikimori",
			"query", filters.Query)

		shikimoriAnimes, err := s.shikimoriClient.SearchAnime(ctx, filters.Query, filters.Page, filters.PageSize)
		if err != nil {
			s.log.Warnw("failed to fetch from Shikimori", "error", err)
			return animes, total, nil // Return empty results
		}

		// Store fetched anime in database
		for _, anime := range shikimoriAnimes {
			if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
				s.log.Warnw("failed to store anime from Shikimori",
					"shikimori_id", anime.ShikimoriID, "error", err)
			}
		}

		// Enrich with genres
		for _, anime := range shikimoriAnimes {
			s.enrichAnime(ctx, anime)
		}

		return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
	}

	return animes, total, nil
}

// GetAnime gets anime by ID
func (s *CatalogService) GetAnime(ctx context.Context, id string) (*domain.Anime, error) {
	// Try cache first
	cacheKey := cache.KeyAnime(id)
	var anime domain.Anime
	if err := s.cache.Get(ctx, cacheKey, &anime); err == nil {
		return &anime, nil
	}

	// Fetch from database
	dbAnime, err := s.animeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.enrichAnime(ctx, dbAnime)

	// Cache the result
	_ = s.cache.Set(ctx, cacheKey, dbAnime, cache.TTLAnimeDetails)

	return dbAnime, nil
}

// GetAnimeByShikimoriID gets or fetches anime by Shikimori ID
func (s *CatalogService) GetAnimeByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error) {
	// Check if we have it locally
	existing, err := s.animeRepo.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		s.enrichAnime(ctx, existing)
		return existing, nil
	}

	// Fetch from Shikimori
	s.log.Infow("fetching anime from Shikimori", "shikimori_id", shikimoriID)

	anime, err := s.shikimoriClient.GetAnimeByID(ctx, shikimoriID)
	if err != nil {
		return nil, err
	}

	// Store in database
	if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
		return nil, fmt.Errorf("store anime: %w", err)
	}

	s.enrichAnime(ctx, anime)
	return anime, nil
}

// GetSeasonalAnime gets anime for a specific season
func (s *CatalogService) GetSeasonalAnime(ctx context.Context, year int, season string, page, pageSize int) ([]*domain.Anime, int64, error) {
	// Try local first
	animes, total, err := s.animeRepo.GetBySeason(ctx, year, season, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// If we have local results, return them
	if len(animes) > 0 {
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return animes, total, nil
	}

	// Fetch from Shikimori
	shikimoriAnimes, err := s.shikimoriClient.GetSeasonalAnime(ctx, year, season, page, pageSize)
	if err != nil {
		s.log.Warnw("failed to fetch seasonal anime from Shikimori", "error", err)
		return animes, total, nil
	}

	// Store fetched anime
	for _, anime := range shikimoriAnimes {
		if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
			s.log.Warnw("failed to store anime", "error", err)
		}
	}

	for _, anime := range shikimoriAnimes {
		s.enrichAnime(ctx, anime)
	}

	return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
}

// GetGenres gets all genres
func (s *CatalogService) GetGenres(ctx context.Context) ([]domain.Genre, error) {
	// Try cache
	cacheKey := "genres:all"
	var genres []domain.Genre
	if err := s.cache.Get(ctx, cacheKey, &genres); err == nil {
		return genres, nil
	}

	genres, err := s.genreRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	_ = s.cache.Set(ctx, cacheKey, genres, cache.TTLGenreList)
	return genres, nil
}

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

// upsertAnimeFromExternal stores or updates anime from external source
func (s *CatalogService) upsertAnimeFromExternal(ctx context.Context, anime *domain.Anime) error {
	// Check if exists
	existing, err := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update existing
		anime.ID = existing.ID
		anime.HasVideo = existing.HasVideo
		anime.CreatedAt = existing.CreatedAt
		return s.animeRepo.Update(ctx, anime)
	}

	// Create new
	if err := s.animeRepo.Create(ctx, anime); err != nil {
		return err
	}

	// Upsert genres
	for _, genre := range anime.Genres {
		if err := s.genreRepo.Upsert(ctx, &genre); err != nil {
			s.log.Warnw("failed to upsert genre", "error", err)
		}
	}

	// Link genres
	genreIDs := make([]string, len(anime.Genres))
	for i, g := range anime.Genres {
		genreIDs[i] = g.ID
	}
	if len(genreIDs) > 0 {
		if err := s.genreRepo.SetAnimeGenres(ctx, anime.ID, genreIDs); err != nil {
			s.log.Warnw("failed to set anime genres", "error", err)
		}
	}

	return nil
}

// enrichAnime adds genres and video sources to anime
func (s *CatalogService) enrichAnime(ctx context.Context, anime *domain.Anime) {
	if anime == nil {
		return
	}

	// Load genres if not already loaded
	if len(anime.Genres) == 0 {
		genres, err := s.genreRepo.GetForAnime(ctx, anime.ID)
		if err == nil {
			anime.Genres = genres
		}
	}

	// Load video sources summary
	videos, err := s.videoRepo.GetForAnime(ctx, anime.ID, "")
	if err == nil && len(videos) > 0 {
		sourceMap := make(map[string]domain.VideoSource)
		for _, v := range videos {
			key := fmt.Sprintf("%s-%s-%s", v.SourceType, v.Quality, v.Language)
			if _, exists := sourceMap[key]; !exists {
				sourceMap[key] = domain.VideoSource{
					Type:     v.SourceType,
					Quality:  v.Quality,
					Language: v.Language,
				}
			}
		}
		for _, vs := range sourceMap {
			anime.VideoSources = append(anime.VideoSources, vs)
		}
	}
}
