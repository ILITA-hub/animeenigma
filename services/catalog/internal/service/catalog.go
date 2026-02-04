package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/aniboom"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/hianime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type CatalogService struct {
	animeRepo       *repo.AnimeRepository
	genreRepo       *repo.GenreRepository
	videoRepo       *repo.VideoRepository
	shikimoriClient *shikimori.Client
	aniboomClient   *aniboom.Client
	kodikClient     *kodik.Client
	hianimeClient   *hianime.Client
	cache           *cache.RedisCache
	log             *logger.Logger
}

// CatalogServiceOptions contains optional configuration for CatalogService
type CatalogServiceOptions struct {
	AniwatchAPIURL string
}

func NewCatalogService(
	animeRepo *repo.AnimeRepository,
	genreRepo *repo.GenreRepository,
	videoRepo *repo.VideoRepository,
	shikimoriClient *shikimori.Client,
	cache *cache.RedisCache,
	log *logger.Logger,
	opts ...CatalogServiceOptions,
) *CatalogService {
	// Initialize Kodik client (log warning if fails, don't block service startup)
	var kodikClient *kodik.Client
	var err error
	kodikClient, err = kodik.NewClient()
	if err != nil {
		log.Warnw("failed to initialize kodik client, kodik features will be unavailable", "error", err)
	}

	// Get Aniwatch API URL from options
	var aniwatchAPIURL string
	if len(opts) > 0 && opts[0].AniwatchAPIURL != "" {
		aniwatchAPIURL = opts[0].AniwatchAPIURL
	}

	return &CatalogService{
		animeRepo:       animeRepo,
		genreRepo:       genreRepo,
		videoRepo:       videoRepo,
		shikimoriClient: shikimoriClient,
		aniboomClient:   aniboom.NewClient(),
		kodikClient:     kodikClient,
		hianimeClient:   hianime.NewClientWithAniwatch(aniwatchAPIURL),
		cache:           cache,
		log:             log,
	}
}

// SearchAnime searches for anime, fetching from Shikimori if not found locally
func (s *CatalogService) SearchAnime(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	// If source=shikimori, force search on Shikimori
	if filters.Source == "shikimori" && filters.Query != "" {
		return s.searchShikimori(ctx, filters)
	}

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
		return s.searchShikimori(ctx, filters)
	}

	return animes, total, nil
}

// searchShikimori fetches anime from Shikimori and stores in DB
func (s *CatalogService) searchShikimori(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	s.log.Infow("fetching from Shikimori",
		"query", filters.Query,
		"forced", filters.Source == "shikimori")

	shikimoriAnimes, err := s.shikimoriClient.SearchAnime(ctx, filters.Query, filters.Page, filters.PageSize)
	if err != nil {
		s.log.Warnw("failed to fetch from Shikimori", "error", err)
		return nil, 0, nil // Return empty results
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

// GetAnimeByMALID gets anime by MAL ID (does not fetch from external)
func (s *CatalogService) GetAnimeByMALID(ctx context.Context, malID string) (*domain.Anime, error) {
	existing, err := s.animeRepo.GetByMALID(ctx, malID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		s.enrichAnime(ctx, existing)
		return existing, nil
	}

	return nil, nil
}

// RefreshAnimeFromShikimori refreshes anime data from Shikimori
func (s *CatalogService) RefreshAnimeFromShikimori(ctx context.Context, animeID string) (*domain.Anime, error) {
	// Get anime from database
	existing, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if existing.ShikimoriID == "" {
		return nil, errors.InvalidInput("anime does not have a shikimori_id")
	}

	s.log.Infow("refreshing anime from Shikimori",
		"anime_id", animeID,
		"shikimori_id", existing.ShikimoriID)

	// Fetch fresh data from Shikimori
	shikimoriAnime, err := s.shikimoriClient.GetAnimeByID(ctx, existing.ShikimoriID)
	if err != nil {
		return nil, fmt.Errorf("fetch from shikimori: %w", err)
	}

	// Preserve local ID and flags
	shikimoriAnime.ID = existing.ID
	shikimoriAnime.HasVideo = existing.HasVideo
	shikimoriAnime.CreatedAt = existing.CreatedAt

	// Update in database
	if err := s.animeRepo.Update(ctx, shikimoriAnime); err != nil {
		return nil, fmt.Errorf("update anime: %w", err)
	}

	// Update genres
	for _, genre := range shikimoriAnime.Genres {
		if err := s.genreRepo.Upsert(ctx, &genre); err != nil {
			s.log.Warnw("failed to upsert genre", "error", err)
		}
	}
	genreIDs := make([]string, len(shikimoriAnime.Genres))
	for i, g := range shikimoriAnime.Genres {
		genreIDs[i] = g.ID
	}
	if len(genreIDs) > 0 {
		if err := s.genreRepo.SetAnimeGenres(ctx, shikimoriAnime.ID, genreIDs); err != nil {
			s.log.Warnw("failed to set anime genres", "error", err)
		}
	}

	// Invalidate cache
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	s.enrichAnime(ctx, shikimoriAnime)
	return shikimoriAnime, nil
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

// GetTrendingAnime gets trending anime (based on recent score/popularity)
func (s *CatalogService) GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	filters := domain.SearchFilters{
		Sort:     "score",
		Order:    "desc",
		Page:     page,
		PageSize: pageSize,
	}

	animes, total, err := s.animeRepo.Search(ctx, filters)
	if err != nil {
		return nil, 0, err
	}

	// If empty, fetch from Shikimori
	if len(animes) == 0 {
		shikimoriAnimes, err := s.shikimoriClient.GetTrendingAnime(ctx, page, pageSize)
		if err != nil {
			s.log.Warnw("failed to fetch trending from Shikimori", "error", err)
			return animes, total, nil
		}

		for _, anime := range shikimoriAnimes {
			if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
				s.log.Warnw("failed to store anime", "error", err)
			}
			s.enrichAnime(ctx, anime)
		}

		return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
	}

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, total, nil
}

// GetPopularAnime gets popular anime (all time)
func (s *CatalogService) GetPopularAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	filters := domain.SearchFilters{
		Sort:     "popularity",
		Order:    "desc",
		Page:     page,
		PageSize: pageSize,
	}

	animes, total, err := s.animeRepo.Search(ctx, filters)
	if err != nil {
		return nil, 0, err
	}

	// If empty, fetch from Shikimori
	if len(animes) == 0 {
		shikimoriAnimes, err := s.shikimoriClient.GetPopularAnime(ctx, page, pageSize)
		if err != nil {
			s.log.Warnw("failed to fetch popular from Shikimori", "error", err)
			return animes, total, nil
		}

		for _, anime := range shikimoriAnimes {
			if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
				s.log.Warnw("failed to store anime", "error", err)
			}
			s.enrichAnime(ctx, anime)
		}

		return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
	}

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, total, nil
}

// GetRecentAnime gets recently added anime
func (s *CatalogService) GetRecentAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	filters := domain.SearchFilters{
		Sort:     "created_at",
		Order:    "desc",
		Page:     page,
		PageSize: pageSize,
	}

	animes, total, err := s.animeRepo.Search(ctx, filters)
	if err != nil {
		return nil, 0, err
	}

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, total, nil
}

// GetSchedule gets anime release schedule (ongoing with next episode dates)
func (s *CatalogService) GetSchedule(ctx context.Context) ([]*domain.Anime, error) {
	animes, err := s.animeRepo.GetSchedule(ctx)
	if err != nil {
		return nil, err
	}

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, nil
}

// GetOngoingAnime gets all ongoing anime
func (s *CatalogService) GetOngoingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	animes, total, err := s.animeRepo.GetOngoingAnime(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, total, nil
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

// GetKodikTranslations gets available translations from Kodik for an anime
func (s *CatalogService) GetKodikTranslations(ctx context.Context, animeID string) ([]domain.KodikTranslation, error) {
	if s.kodikClient == nil {
		s.log.Warnw("kodik client not initialized, returning empty translations",
			"anime_id", animeID)
		return []domain.KodikTranslation{}, nil
	}

	// Get anime to get Shikimori ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if anime.ShikimoriID == "" {
		s.log.Warnw("anime does not have shikimori_id, cannot fetch kodik translations",
			"anime_id", animeID)
		return []domain.KodikTranslation{}, nil
	}

	// Check cache first
	cacheKey := fmt.Sprintf("kodik:translations:%s", animeID)
	var cached []domain.KodikTranslation
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Get translations from Kodik (with retry on failure)
	translations, err := s.kodikClient.GetTranslations(anime.ShikimoriID)
	if err != nil {
		s.log.Warnw("failed to get kodik translations, returning empty list",
			"anime_id", animeID,
			"shikimori_id", anime.ShikimoriID,
			"error", err)
		// Return empty list instead of error - Kodik is not critical
		return []domain.KodikTranslation{}, nil
	}

	result := make([]domain.KodikTranslation, len(translations))
	for i, t := range translations {
		result[i] = domain.KodikTranslation{
			ID:            t.ID,
			Title:         t.Title,
			Type:          t.Type,
			EpisodesCount: t.EpisodesCount,
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetKodikVideoSource gets video embed link from Kodik for a specific episode
func (s *CatalogService) GetKodikVideoSource(ctx context.Context, animeID string, episode int, translationID int) (*domain.KodikVideoSource, error) {
	if s.kodikClient == nil {
		return nil, errors.NotFound("kodik not available")
	}

	// Get anime to get Shikimori ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if anime.ShikimoriID == "" {
		return nil, errors.NotFound("anime does not have shikimori_id")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("kodik:video:%s:%d:%d", animeID, episode, translationID)
	var cached domain.KodikVideoSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	// Get episode link from Kodik
	embedLink, err := s.kodikClient.GetEpisodeLink(anime.ShikimoriID, episode, translationID)
	if err != nil {
		s.log.Warnw("failed to get kodik video source",
			"anime_id", animeID,
			"shikimori_id", anime.ShikimoriID,
			"episode", episode,
			"translation_id", translationID,
			"error", err)
		return nil, errors.NotFound("video not found on kodik")
	}

	// Get translation name
	translationName := ""
	translations, _ := s.kodikClient.GetTranslations(anime.ShikimoriID)
	for _, t := range translations {
		if t.ID == translationID {
			translationName = t.Title
			break
		}
	}

	result := &domain.KodikVideoSource{
		EmbedLink:     embedLink,
		Episode:       episode,
		TranslationID: translationID,
		Translation:   translationName,
	}

	// Cache for 1 hour (embed links are relatively stable)
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// SearchKodik searches for anime on Kodik by title
func (s *CatalogService) SearchKodik(ctx context.Context, title string) ([]domain.KodikSearchResult, error) {
	if s.kodikClient == nil {
		return nil, errors.Internal("kodik client not initialized")
	}

	results, err := s.kodikClient.SearchByTitle(title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.KodikSearchResult, 0, len(results))
	seen := make(map[string]bool)

	for _, r := range results {
		// Deduplicate by title (different translations appear as separate results)
		if seen[r.Title] {
			continue
		}
		seen[r.Title] = true

		var translation *domain.KodikTranslation
		if r.Translation != nil {
			translation = &domain.KodikTranslation{
				ID:    r.Translation.ID,
				Title: r.Translation.Title,
				Type:  r.Translation.Type,
			}
		}

		searchResults = append(searchResults, domain.KodikSearchResult{
			ID:            r.ID,
			Type:          r.Type,
			Link:          r.Link,
			Title:         r.Title,
			TitleOrig:     r.TitleOrig,
			Year:          r.Year,
			EpisodesCount: r.EpisodesCount,
			ShikimoriID:   r.ShikimoriID,
			Translation:   translation,
			Quality:       r.Quality,
		})
	}

	return searchResults, nil
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

	for _, anime := range animes {
		s.enrichAnime(ctx, anime)
	}

	return animes, nil
}

// UpdateShikimoriID updates the Shikimori ID for an anime (admin only)
func (s *CatalogService) UpdateShikimoriID(ctx context.Context, animeID string, shikimoriID string) error {
	// Invalidate cache for kodik translations
	_ = s.cache.Delete(ctx, fmt.Sprintf("kodik:translations:%s", animeID))
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	return s.animeRepo.UpdateShikimoriID(ctx, animeID, shikimoriID)
}

// GetHiAnimeEpisodes gets episodes from HiAnime for an anime
func (s *CatalogService) GetHiAnimeEpisodes(ctx context.Context, animeID string) ([]domain.HiAnimeEpisode, error) {
	// Get anime to search by name
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("hianime:episodes:%s", animeID)
	var cached []domain.HiAnimeEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Search for anime on HiAnime by name
	hiAnimeID, err := s.findHiAnimeID(ctx, anime)
	if err != nil {
		s.log.Warnw("failed to find anime on hianime",
			"anime_id", animeID,
			"name", anime.Name,
			"error", err)
		return []domain.HiAnimeEpisode{}, nil
	}

	// Get episodes
	episodes, err := s.hianimeClient.GetEpisodes(hiAnimeID)
	if err != nil {
		s.log.Warnw("failed to get hianime episodes",
			"anime_id", animeID,
			"hianime_id", hiAnimeID,
			"error", err)
		return []domain.HiAnimeEpisode{}, nil
	}

	result := make([]domain.HiAnimeEpisode, len(episodes))
	for i, ep := range episodes {
		result[i] = domain.HiAnimeEpisode{
			ID:       ep.ID,
			Number:   ep.Number,
			Title:    ep.Title,
			IsFiller: ep.IsFiller,
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetHiAnimeServers gets available servers for an episode from HiAnime
func (s *CatalogService) GetHiAnimeServers(ctx context.Context, animeID string, episodeID string) ([]domain.HiAnimeServer, error) {
	// URL-decode the episode ID in case it was encoded in the request
	decodedEpisodeID, err := url.QueryUnescape(episodeID)
	if err == nil && decodedEpisodeID != episodeID {
		episodeID = decodedEpisodeID
	}

	// Check cache first
	cacheKey := fmt.Sprintf("hianime:servers:%s:%s", animeID, episodeID)
	var cached []domain.HiAnimeServer
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	servers, err := s.hianimeClient.GetServers(episodeID)
	if err != nil {
		s.log.Warnw("failed to get hianime servers",
			"anime_id", animeID,
			"episode_id", episodeID,
			"error", err)
		return nil, errors.NotFound("servers not available")
	}

	result := make([]domain.HiAnimeServer, len(servers))
	for i, srv := range servers {
		result[i] = domain.HiAnimeServer{
			ID:   srv.ID,
			Name: srv.Name,
			Type: srv.Type,
		}
	}

	// Cache for 30 minutes
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)

	return result, nil
}

// GetHiAnimeStream gets the stream URL from HiAnime
func (s *CatalogService) GetHiAnimeStream(ctx context.Context, animeID string, episodeID string, serverID string, category string) (*domain.HiAnimeStream, error) {
	// URL-decode the episode ID in case it was encoded in the request
	decodedEpisodeID, err := url.QueryUnescape(episodeID)
	if err == nil && decodedEpisodeID != episodeID {
		episodeID = decodedEpisodeID
	}

	// Check cache first
	cacheKey := fmt.Sprintf("hianime:stream:%s:%s:%s:%s", animeID, episodeID, serverID, category)
	var cached domain.HiAnimeStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	stream, err := s.hianimeClient.GetStream(episodeID, serverID, category)
	if err != nil {
		s.log.Warnw("failed to get hianime stream",
			"anime_id", animeID,
			"episode_id", episodeID,
			"server_id", serverID,
			"category", category,
			"error", err)
		// Return detailed error message to user
		return nil, errors.NotFound(fmt.Sprintf("Stream unavailable: %s", err.Error()))
	}

	// Convert subtitles
	var subtitles []domain.HiAnimeSubtitle
	for _, sub := range stream.Subtitles {
		subtitles = append(subtitles, domain.HiAnimeSubtitle{
			URL:     sub.URL,
			Lang:    sub.Lang,
			Label:   sub.Label,
			Default: sub.Default,
		})
	}

	result := &domain.HiAnimeStream{
		URL:       stream.URL,
		Type:      stream.Type,
		Subtitles: subtitles,
		Headers:   stream.Headers,
	}

	if stream.Intro != nil {
		result.Intro = &domain.HiAnimeTimeRange{
			Start: stream.Intro.Start,
			End:   stream.Intro.End,
		}
	}
	if stream.Outro != nil {
		result.Outro = &domain.HiAnimeTimeRange{
			Start: stream.Outro.Start,
			End:   stream.Outro.End,
		}
	}

	// Cache for 30 minutes (stream URLs may expire)
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)

	return result, nil
}

// SearchHiAnime searches for anime on HiAnime by title
func (s *CatalogService) SearchHiAnime(ctx context.Context, title string) ([]domain.HiAnimeSearchResult, error) {
	results, err := s.hianimeClient.Search(title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.HiAnimeSearchResult, len(results))
	for i, r := range results {
		searchResults[i] = domain.HiAnimeSearchResult{
			ID:       r.ID,
			Name:     r.Name,
			Poster:   r.Poster,
			Type:     r.Type,
			Duration: r.Duration,
		}
	}

	return searchResults, nil
}

// findHiAnimeID finds the HiAnime ID for an anime by searching
func (s *CatalogService) findHiAnimeID(ctx context.Context, anime *domain.Anime) (string, error) {
	// Check cache for HiAnime ID mapping
	cacheKey := fmt.Sprintf("hianime:mapping:%s", anime.ID)
	var cachedID string
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil && cachedID != "" {
		return cachedID, nil
	}

	// Search by different name variants
	searchNames := []string{}
	if anime.Name != "" {
		searchNames = append(searchNames, anime.Name)
	}
	if anime.NameRU != "" && anime.NameRU != anime.Name {
		searchNames = append(searchNames, anime.NameRU)
	}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	for _, name := range searchNames {
		results, err := s.hianimeClient.Search(name)
		if err != nil {
			continue
		}

		// Find best match
		for _, r := range results {
			// Simple matching - could be improved with fuzzy matching
			if matchesAnime(r.Name, anime) {
				// Cache the mapping for 24 hours
				_ = s.cache.Set(ctx, cacheKey, r.ID, 24*time.Hour)
				return r.ID, nil
			}
		}

		// If no exact match, use the first result if available
		if len(results) > 0 {
			_ = s.cache.Set(ctx, cacheKey, results[0].ID, 24*time.Hour)
			return results[0].ID, nil
		}
	}

	return "", fmt.Errorf("anime not found on HiAnime")
}

// matchesAnime checks if a search result matches an anime
func matchesAnime(resultName string, anime *domain.Anime) bool {
	resultLower := normalizeTitle(resultName)

	if anime.Name != "" && normalizeTitle(anime.Name) == resultLower {
		return true
	}
	if anime.NameRU != "" && normalizeTitle(anime.NameRU) == resultLower {
		return true
	}
	if anime.NameJP != "" && normalizeTitle(anime.NameJP) == resultLower {
		return true
	}

	return false
}

// normalizeTitle normalizes a title for comparison
func normalizeTitle(title string) string {
	// Convert to lowercase and remove common variations
	title = strings.ToLower(title)
	title = strings.TrimSpace(title)
	// Remove special characters
	title = strings.ReplaceAll(title, ":", "")
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "  ", " ")
	return title
}
