package service

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/aniboom"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/consumet"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/hianime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jikan"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// hianimeInflight deduplicates concurrent findHiAnimeID lookups for the same anime
type hianimeInflight struct {
	done chan struct{}
	id   string
	err  error
}

type CatalogService struct {
	animeRepo       *repo.AnimeRepository
	genreRepo       *repo.GenreRepository
	videoRepo       *repo.VideoRepository
	shikimoriClient *shikimori.Client
	aniboomClient   *aniboom.Client
	kodikClient     *kodik.Client
	hianimeClient   *hianime.Client
	consumetClient  *consumet.Client
	jikanClient     *jikan.Client
	jimakuClient    *jimaku.Client
	animelibClient  *animelib.Client
	idMappingClient *idmapping.Client
	cache           *cache.RedisCache
	log             *logger.Logger

	hianimeLookups sync.Map // map[string]*hianimeInflight
}

// CatalogServiceOptions contains optional configuration for CatalogService
type CatalogServiceOptions struct {
	AniwatchAPIURL string
	ConsumetAPIURL string
	JimakuAPIKey   string
	AnimeLibToken  string
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

	// Get API URLs from options
	var aniwatchAPIURL, consumetAPIURL, jimakuAPIKey, animelibToken string
	if len(opts) > 0 {
		aniwatchAPIURL = opts[0].AniwatchAPIURL
		consumetAPIURL = opts[0].ConsumetAPIURL
		jimakuAPIKey = opts[0].JimakuAPIKey
		animelibToken = opts[0].AnimeLibToken
	}

	jimakuClient := jimaku.NewClient(jimakuAPIKey)
	if jimakuClient.IsConfigured() {
		log.Infow("jimaku client initialized")
	} else {
		log.Warnw("jimaku client not configured, japanese subtitle features will be unavailable")
	}

	animelibClient := animelib.NewClient(animelibToken)
	if animelibToken != "" {
		log.Infow("animelib client initialized with token")
	} else {
		log.Infow("animelib client initialized without token")
	}

	return &CatalogService{
		animeRepo:       animeRepo,
		genreRepo:       genreRepo,
		videoRepo:       videoRepo,
		shikimoriClient: shikimoriClient,
		aniboomClient:   aniboom.NewClient(),
		kodikClient:     kodikClient,
		hianimeClient:   hianime.NewClientWithAniwatch(aniwatchAPIURL),
		consumetClient:  consumet.NewClient(consumetAPIURL),
		jikanClient:     jikan.NewClient(),
		jimakuClient:    jimakuClient,
		animelibClient:  animelibClient,
		idMappingClient: idmapping.NewClient(),
		cache:           cache,
		log:             log,
	}
}

// KodikClient returns the Kodik parser client (may be nil if init failed).
func (s *CatalogService) KodikClient() *kodik.Client {
	return s.kodikClient
}

// AnimeLibClient returns the AnimeLib parser client.
func (s *CatalogService) AnimeLibClient() *animelib.Client {
	return s.animelibClient
}

// SearchAnime searches for anime, fetching from Shikimori if not found locally
func (s *CatalogService) SearchAnime(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	// If source=shikimori, force search on Shikimori (skip cache)
	if filters.Source == "shikimori" && filters.Query != "" {
		metrics.SearchRequestsTotal.WithLabelValues("shikimori").Inc()
		return s.searchShikimori(ctx, filters)
	}

	// Check search result cache for simple query searches
	var searchCacheKey string
	if filters.Query != "" {
		searchCacheKey = cache.KeySearchResults(filters.Query, filters.Page)
		var cached struct {
			Animes []*domain.Anime `json:"animes"`
			Total  int64           `json:"total"`
		}
		if err := s.cache.Get(ctx, searchCacheKey, &cached); err == nil {
			metrics.SearchRequestsTotal.WithLabelValues("cache").Inc()
			return cached.Animes, cached.Total, nil
		}
	}

	// First, try to search locally
	animes, total, err := s.animeRepo.Search(ctx, filters)
	if err != nil {
		return nil, 0, err
	}

	// If we have local results, enrich and return them
	if len(animes) > 0 {
		metrics.SearchRequestsTotal.WithLabelValues("local_db").Inc()
		s.enrichAnimesBatch(ctx, animes)
		// Cache the result
		if searchCacheKey != "" {
			_ = s.cache.Set(ctx, searchCacheKey, struct {
				Animes []*domain.Anime `json:"animes"`
				Total  int64           `json:"total"`
			}{Animes: animes, Total: total}, cache.TTLSearchResults)
		}
		return animes, total, nil
	}

	// No local results - fetch from Shikimori
	if filters.Query != "" {
		metrics.SearchRequestsTotal.WithLabelValues("shikimori").Inc()
		shikiAnimes, shikiTotal, shikiErr := s.searchShikimori(ctx, filters)
		if shikiErr == nil && len(shikiAnimes) > 0 && searchCacheKey != "" {
			_ = s.cache.Set(ctx, searchCacheKey, struct {
				Animes []*domain.Anime `json:"animes"`
				Total  int64           `json:"total"`
			}{Animes: shikiAnimes, Total: shikiTotal}, cache.TTLSearchResults)
		}
		return shikiAnimes, shikiTotal, shikiErr
	}

	return animes, total, nil
}

// searchShikimori fetches anime from Shikimori and stores in DB
func (s *CatalogService) searchShikimori(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	s.log.Infow("fetching from Shikimori",
		"query", filters.Query,
		"forced", filters.Source == "shikimori")

	shikiStart := time.Now()
	shikimoriAnimes, err := s.shikimoriClient.SearchAnime(ctx, filters.Query, filters.Page, filters.PageSize)
	metrics.ExternalAPIDuration.WithLabelValues("shikimori").Observe(time.Since(shikiStart).Seconds())
	if err != nil {
		metrics.ExternalAPIRequestsTotal.WithLabelValues("shikimori", "error").Inc()
		s.log.Warnw("failed to fetch from Shikimori", "error", err)
		return nil, 0, nil // Return empty results
	}
	metrics.ExternalAPIRequestsTotal.WithLabelValues("shikimori", "success").Inc()

	// Store fetched anime in database
	for _, anime := range shikimoriAnimes {
		if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
			s.log.Warnw("failed to store anime from Shikimori",
				"shikimori_id", anime.ShikimoriID, "error", err)
		}
	}

	// Enrich with genres and video sources (batch)
	s.enrichAnimesBatch(ctx, shikimoriAnimes)

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

// ResolveMALAnime resolves a MAL ID to a local anime record.
// First tries direct Shikimori lookup (since Shikimori IDs = MAL IDs),
// then falls back to Jikan title matching. Returns "resolved" with anime
// if found, or "ambiguous" with the MAL title for manual search.
func (s *CatalogService) ResolveMALAnime(ctx context.Context, malID string) (*domain.MALResolveResult, error) {
	// Step 1: Check local DB first (by mal_id)
	existing, err := s.animeRepo.GetByMALID(ctx, malID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		s.enrichAnime(ctx, existing)
		return &domain.MALResolveResult{
			Status: "resolved",
			Anime:  existing,
			MALID:  malID,
		}, nil
	}

	// Step 2: Shikimori IDs = MAL IDs, so try direct Shikimori lookup
	// This avoids Jikan rate limits and is deterministic (no title matching)
	anime, err := s.GetAnimeByShikimoriID(ctx, malID)
	if err == nil && anime != nil {
		// Backfill MAL ID if not already set
		if anime.MALID == "" {
			anime.MALID = malID
			_ = s.animeRepo.UpdateMALID(ctx, anime.ID, malID)
		}
		return &domain.MALResolveResult{
			Status: "resolved",
			Anime:  anime,
			MALID:  malID,
		}, nil
	}
	if err != nil {
		s.log.Warnw("Shikimori direct lookup failed, falling back to Jikan",
			"mal_id", malID, "error", err)
	}

	// Step 3: Fall back to Jikan title matching (for IDs Shikimori doesn't recognize)
	s.log.Infow("resolving MAL ID via Jikan", "mal_id", malID)

	malInfo, err := s.jikanClient.GetAnimeByID(ctx, malID)
	if err != nil {
		s.log.Warnw("failed to fetch MAL info via Jikan", "mal_id", malID, "error", err)
		return &domain.MALResolveResult{
			Status: "ambiguous",
			MALID:  malID,
		}, nil
	}

	// Step 4: Search Shikimori by romanized title
	searchTitle := malInfo.Title
	if searchTitle == "" {
		searchTitle = malInfo.TitleEnglish
	}

	shikimoriAnimes, err := s.shikimoriClient.SearchAnime(ctx, searchTitle, 1, 10)
	if err != nil {
		s.log.Warnw("Shikimori search failed during MAL resolution",
			"mal_id", malID, "query", searchTitle, "error", err)
		return &domain.MALResolveResult{
			Status:   "ambiguous",
			MALTitle: searchTitle,
			MALID:    malID,
		}, nil
	}

	// Step 5: Look for an exact name match
	var matched *domain.Anime
	for _, anime := range shikimoriAnimes {
		if titlesMatch(anime.Name, malInfo.Title) ||
			titlesMatch(anime.NameJP, malInfo.TitleJapanese) ||
			titlesMatch(anime.Name, malInfo.TitleEnglish) {
			matched = anime
			break
		}
	}

	if matched == nil {
		return &domain.MALResolveResult{
			Status:   "ambiguous",
			MALTitle: searchTitle,
			MALID:    malID,
		}, nil
	}

	// Step 6: Store matched anime, backfill MAL ID
	if err := s.upsertAnimeFromExternal(ctx, matched); err != nil {
		return nil, fmt.Errorf("store resolved anime: %w", err)
	}
	if matched.MALID == "" {
		matched.MALID = malID
		_ = s.animeRepo.UpdateMALID(ctx, matched.ID, malID)
	}

	s.enrichAnime(ctx, matched)
	return &domain.MALResolveResult{
		Status: "resolved",
		Anime:  matched,
		MALID:  malID,
	}, nil
}

// titlesMatch compares two titles case-insensitively after normalization
func titlesMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return normalizeTitle(a) == normalizeTitle(b)
}

// GetAnimeByMALID is a backward-compatible wrapper for ResolveMALAnime.
// Returns the anime if resolved, or nil if ambiguous.
func (s *CatalogService) GetAnimeByMALID(ctx context.Context, malID string) (*domain.Anime, error) {
	result, err := s.ResolveMALAnime(ctx, malID)
	if err != nil {
		return nil, err
	}
	if result.Status == "resolved" {
		return result.Anime, nil
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

// BatchRefreshAnime refreshes all stale anime of a given status using batch Shikimori queries.
// Returns counts of refreshed and failed anime.
func (s *CatalogService) BatchRefreshAnime(ctx context.Context, status domain.AnimeStatus, staleBefore time.Time) (refreshed, failed int, err error) {
	staleAnime, err := s.animeRepo.GetStaleAnime(ctx, status, staleBefore)
	if err != nil {
		return 0, 0, err
	}

	if len(staleAnime) == 0 {
		s.log.Infow("no stale anime to refresh", "status", status)
		return 0, 0, nil
	}

	s.log.Infow("batch refreshing stale anime",
		"status", status,
		"count", len(staleAnime),
	)

	// Build shikimoriID -> existing anime map
	existingMap := make(map[string]*domain.Anime, len(staleAnime))
	var shikimoriIDs []string
	for _, a := range staleAnime {
		existingMap[a.ShikimoriID] = a
		shikimoriIDs = append(shikimoriIDs, a.ShikimoriID)
	}

	// Process in chunks of 50
	const batchSize = 50
	for i := 0; i < len(shikimoriIDs); i += batchSize {
		select {
		case <-ctx.Done():
			return refreshed, failed, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(shikimoriIDs) {
			end = len(shikimoriIDs)
		}
		batch := shikimoriIDs[i:end]

		s.log.Infow("fetching batch from Shikimori",
			"batch", fmt.Sprintf("%d-%d/%d", i+1, end, len(shikimoriIDs)),
			"status", status,
		)

		freshAnime, err := s.shikimoriClient.GetAnimeByIDs(ctx, batch)
		if err != nil {
			s.log.Warnw("batch fetch failed", "error", err, "batch_start", i)
			failed += len(batch)
			continue
		}

		for _, fresh := range freshAnime {
			existing, ok := existingMap[fresh.ShikimoriID]
			if !ok {
				continue
			}

			// Preserve local fields
			fresh.ID = existing.ID
			fresh.HasVideo = existing.HasVideo
			fresh.CreatedAt = existing.CreatedAt

			if err := s.animeRepo.Update(ctx, fresh); err != nil {
				s.log.Warnw("failed to update anime", "id", existing.ID, "error", err)
				failed++
				continue
			}

			// Upsert genres
			for _, genre := range fresh.Genres {
				_ = s.genreRepo.Upsert(ctx, &genre)
			}
			genreIDs := make([]string, len(fresh.Genres))
			for j, g := range fresh.Genres {
				genreIDs[j] = g.ID
			}
			if len(genreIDs) > 0 {
				_ = s.genreRepo.SetAnimeGenres(ctx, fresh.ID, genreIDs)
			}

			// Invalidate cache
			_ = s.cache.Delete(ctx, cache.KeyAnime(existing.ID))
			refreshed++
		}

		// Rate limit safety between batches
		if end < len(shikimoriIDs) {
			time.Sleep(200 * time.Millisecond)
		}
	}

	s.log.Infow("batch refresh completed",
		"status", status,
		"refreshed", refreshed,
		"failed", failed,
	)
	return refreshed, failed, nil
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

// GetTrendingAnime gets trending anime from cache (backed by Shikimori).
// Cache-first: returns cached top anime (24h TTL), on miss fetches from Shikimori and caches.
func (s *CatalogService) GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	// Only cache page 1 (the main top anime list)
	if page == 1 {
		var cached []*domain.Anime
		if err := s.cache.Get(ctx, cache.KeyTopAnime(), &cached); err == nil && len(cached) > 0 {
			// Slice to requested page size
			result := cached
			if pageSize < len(result) {
				result = result[:pageSize]
			}
			for _, anime := range result {
				s.enrichAnime(ctx, anime)
			}
			return result, int64(len(cached)), nil
		}
	}

	// Cache miss — fetch from Shikimori
	shikimoriAnimes, err := s.shikimoriClient.GetTrendingAnime(ctx, page, pageSize)
	if err != nil {
		s.log.Warnw("failed to fetch trending from Shikimori, falling back to local DB", "error", err)
		// Fallback to local DB sort
		filters := domain.SearchFilters{
			Sort:     "score",
			Order:    "desc",
			Page:     page,
			PageSize: pageSize,
		}
		animes, total, dbErr := s.animeRepo.Search(ctx, filters)
		if dbErr != nil {
			return nil, 0, dbErr
		}
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return animes, total, nil
	}

	// Upsert to DB and cache
	for _, anime := range shikimoriAnimes {
		if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
			s.log.Warnw("failed to store trending anime", "error", err)
		}
		s.enrichAnime(ctx, anime)
	}

	// Cache page 1 results for 24 hours
	if page == 1 && len(shikimoriAnimes) > 0 {
		if err := s.cache.Set(ctx, cache.KeyTopAnime(), shikimoriAnimes, cache.TTLTopAnime); err != nil {
			s.log.Warnw("failed to cache top anime", "error", err)
		}
	}

	return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
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

// enrichAnimesBatch loads genres and video sources for multiple anime in bulk (2 queries instead of N*2).
func (s *CatalogService) enrichAnimesBatch(ctx context.Context, animes []*domain.Anime) {
	if len(animes) == 0 {
		return
	}

	ids := make([]string, len(animes))
	for i, a := range animes {
		ids[i] = a.ID
	}

	genresMap, err := s.genreRepo.GetForAnimes(ctx, ids)
	if err != nil {
		s.log.Warnw("batch genre load failed, falling back to individual", "error", err)
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return
	}

	videosMap, err := s.videoRepo.GetForAnimes(ctx, ids)
	if err != nil {
		s.log.Warnw("batch video load failed, falling back to individual", "error", err)
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return
	}

	for _, anime := range animes {
		if len(anime.Genres) == 0 {
			anime.Genres = genresMap[anime.ID]
		}
		videos := videosMap[anime.ID]
		if len(videos) > 0 {
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
func (s *CatalogService) GetKodikTranslations(ctx context.Context, animeID string) (_ []domain.KodikTranslation, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_translations", start, &retErr)
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
func (s *CatalogService) GetKodikVideoSource(ctx context.Context, animeID string, episode int, translationID int) (_ *domain.KodikVideoSource, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("kodik").Inc()
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

// GetHiAnimeEpisodes gets episodes from HiAnime for an anime
func (s *CatalogService) GetHiAnimeEpisodes(ctx context.Context, animeID string) (retEps []domain.HiAnimeEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hianime", "get_episodes", start, &retErr)

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
func (s *CatalogService) GetHiAnimeServers(ctx context.Context, animeID string, episodeID string) (_ []domain.HiAnimeServer, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hianime", "get_servers", start, &retErr)
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
func (s *CatalogService) GetHiAnimeStream(ctx context.Context, animeID string, episodeID string, serverID string, category string) (_ *domain.HiAnimeStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hianime", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("hianime").Inc()
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

	// Persist AniList ID if available and not already set
	if stream.AnilistID > 0 {
		anime, aErr := s.animeRepo.GetByID(ctx, animeID)
		if aErr == nil && anime != nil && anime.AniListID == "" {
			anilistStr := strconv.Itoa(stream.AnilistID)
			if uErr := s.animeRepo.UpdateAniListID(ctx, animeID, anilistStr); uErr == nil {
				s.log.Infow("saved AniList ID from HiAnime stream",
					"anime_id", animeID,
					"anilist_id", anilistStr)
			}
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

// findHiAnimeID finds the HiAnime ID for an anime by searching.
// Uses singleflight to deduplicate concurrent lookups for the same anime.
func (s *CatalogService) findHiAnimeID(ctx context.Context, anime *domain.Anime) (string, error) {
	cacheKey := fmt.Sprintf("hianime:mapping:%s", anime.ID)

	// Check cache first (includes negative cache)
	var cachedID string
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil {
		if cachedID == "" {
			return "", fmt.Errorf("anime not found on HiAnime (cached)")
		}
		return cachedID, nil
	}

	// Singleflight: deduplicate concurrent lookups for same anime
	flight := &hianimeInflight{done: make(chan struct{})}
	if existing, loaded := s.hianimeLookups.LoadOrStore(anime.ID, flight); loaded {
		// Another goroutine is already looking this up — wait for its result
		existing := existing.(*hianimeInflight)
		<-existing.done
		return existing.id, existing.err
	}

	// We own this lookup — do the search
	defer func() {
		close(flight.done)
		s.hianimeLookups.Delete(anime.ID)
	}()

	id, err := s.doHiAnimeSearch(ctx, anime, cacheKey)
	flight.id = id
	flight.err = err
	return id, err
}

// doHiAnimeSearch performs the actual HiAnime search across name variants in parallel.
func (s *CatalogService) doHiAnimeSearch(ctx context.Context, anime *domain.Anime, cacheKey string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	searchNames := []string{}
	var extraMatchNames []string // additional names for matching (not from anime DB)

	if anime.Name != "" {
		searchNames = append(searchNames, anime.Name)
	}

	// Fetch English title from Jikan (cached) — HiAnime uses English titles while Shikimori gives romanized Japanese
	if anime.MALID != "" {
		jikanCacheKey := fmt.Sprintf("jikan:title:%s", anime.MALID)
		var jikanInfo jikan.AnimeInfo
		if err := s.cache.Get(ctx, jikanCacheKey, &jikanInfo); err != nil {
			fetched, fetchErr := s.jikanClient.GetAnimeByID(ctx, anime.MALID)
			if fetchErr == nil && fetched != nil {
				jikanInfo = *fetched
				_ = s.cache.Set(ctx, jikanCacheKey, jikanInfo, 7*24*time.Hour) // stable data
			}
		}
		if jikanInfo.TitleEnglish != "" && jikanInfo.TitleEnglish != anime.Name {
			searchNames = append(searchNames, jikanInfo.TitleEnglish)
			extraMatchNames = append(extraMatchNames, jikanInfo.TitleEnglish)
		}
	}

	if anime.NameRU != "" && anime.NameRU != anime.Name {
		searchNames = append(searchNames, anime.NameRU)
	}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	if len(searchNames) == 0 {
		_ = s.cache.Set(ctx, cacheKey, "", 10*time.Minute)
		return "", fmt.Errorf("anime not found on HiAnime")
	}

	// Launch all name-variant searches in parallel, return first match
	type searchResult struct{ id string }
	ch := make(chan searchResult, 1)
	var wg sync.WaitGroup

	for _, name := range searchNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			results, err := s.hianimeClient.Search(name)
			if err != nil {
				s.log.Debugw("hianime search failed for variant", "name", name, "error", err)
				return
			}
			for _, r := range results {
				if matchesAnime(r.Name, anime, extraMatchNames...) ||
					(r.JName != "" && matchesAnime(r.JName, anime, extraMatchNames...)) {
					select {
					case ch <- searchResult{id: r.ID}:
					default:
					}
					return
				}
			}
		}(name)
	}

	// Close channel when all goroutines finish
	go func() {
		wg.Wait()
		close(ch)
	}()

	if found, ok := <-ch; ok {
		cancel() // stop remaining goroutines from starting new work
		_ = s.cache.Set(ctx, cacheKey, found.id, 6*time.Hour)
		return found.id, nil
	}

	// Cache negative result for 10 minutes to avoid hammering HiAnime
	_ = s.cache.Set(ctx, cacheKey, "", 10*time.Minute)
	return "", fmt.Errorf("anime not found on HiAnime")
}

// matchesAnime checks if a search result matches an anime using exact and containment matching.
// Extra names (e.g. English title from Jikan) can be passed for additional matching.
func matchesAnime(resultName string, anime *domain.Anime, extraNames ...string) bool {
	resultNorm := normalizeTitle(resultName)

	names := []string{anime.Name, anime.NameRU, anime.NameJP}
	names = append(names, extraNames...)
	for _, name := range names {
		if name == "" {
			continue
		}
		norm := normalizeTitle(name)
		// Exact match
		if norm == resultNorm {
			return true
		}
		// Containment match (bidirectional) — only for names longer than 3 chars to avoid false positives
		if len(norm) > 3 && len(resultNorm) > 3 {
			if strings.Contains(resultNorm, norm) || strings.Contains(norm, resultNorm) {
				return true
			}
		}
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

// ============================================================================
// Consumet API Methods
// ============================================================================

// GetConsumetEpisodes gets episodes from Consumet for an anime
func (s *CatalogService) GetConsumetEpisodes(ctx context.Context, animeID string) (_ []domain.ConsumetEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("consumet", "get_episodes", start, &retErr)
	// Get anime to search by name
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("consumet:episodes:%s", animeID)
	var cached []domain.ConsumetEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Find anime on Consumet
	consumetID, err := s.findConsumetID(ctx, anime)
	if err != nil {
		s.log.Warnw("failed to find anime on consumet",
			"anime_id", animeID,
			"name", anime.Name,
			"error", err)
		return []domain.ConsumetEpisode{}, nil
	}

	// Get episodes
	episodes, err := s.consumetClient.GetEpisodes(consumetID)
	if err != nil {
		s.log.Warnw("failed to get consumet episodes",
			"anime_id", animeID,
			"consumet_id", consumetID,
			"error", err)
		return []domain.ConsumetEpisode{}, nil
	}

	result := make([]domain.ConsumetEpisode, len(episodes))
	for i, ep := range episodes {
		result[i] = domain.ConsumetEpisode{
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

// GetConsumetServers gets available servers from Consumet
func (s *CatalogService) GetConsumetServers(ctx context.Context) []domain.ConsumetServer {
	servers := s.consumetClient.GetServers()
	result := make([]domain.ConsumetServer, len(servers))
	for i, srv := range servers {
		result[i] = domain.ConsumetServer{
			Name: srv.Name,
		}
	}
	return result
}

// GetConsumetStream gets the stream URL from Consumet
func (s *CatalogService) GetConsumetStream(ctx context.Context, animeID string, episodeID string, serverName string) (_ *domain.ConsumetStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("consumet", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("consumet").Inc()
	// Check cache first
	cacheKey := fmt.Sprintf("consumet:stream:%s:%s:%s", animeID, episodeID, serverName)
	var cached domain.ConsumetStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	stream, err := s.consumetClient.GetStream(episodeID, serverName)
	if err != nil {
		s.log.Warnw("failed to get consumet stream",
			"anime_id", animeID,
			"episode_id", episodeID,
			"server", serverName,
			"error", err)
		return nil, errors.NotFound(fmt.Sprintf("Stream unavailable: %s", err.Error()))
	}

	if len(stream.Sources) == 0 {
		return nil, errors.NotFound("no stream sources available")
	}

	// Get the best quality source (prefer 1080p > 720p > others)
	source := stream.Sources[0]
	for _, s := range stream.Sources {
		if strings.Contains(s.Quality, "1080p") || s.Quality == "auto" {
			source = s
			break
		}
		if strings.Contains(s.Quality, "720p") && !strings.Contains(source.Quality, "1080p") {
			source = s
		}
	}

	// Convert all sources for quality selection
	var allSources []domain.ConsumetSource
	for _, s := range stream.Sources {
		allSources = append(allSources, domain.ConsumetSource{
			URL:     s.URL,
			Quality: s.Quality,
			IsM3U8:  s.IsM3U8,
		})
	}

	// Convert subtitles
	var subtitles []domain.ConsumetSubtitle
	for _, sub := range stream.Subtitles {
		subtitles = append(subtitles, domain.ConsumetSubtitle{
			URL:  sub.URL,
			Lang: sub.Lang,
		})
	}

	result := &domain.ConsumetStream{
		URL:       source.URL,
		IsM3U8:    source.IsM3U8,
		Quality:   source.Quality,
		Sources:   allSources,
		Headers:   stream.Headers,
		Subtitles: subtitles,
	}

	// Cache for 30 minutes
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)

	return result, nil
}

// SearchConsumet searches for anime on Consumet by title
func (s *CatalogService) SearchConsumet(ctx context.Context, title string) ([]domain.ConsumetSearchResult, error) {
	results, err := s.consumetClient.Search(title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.ConsumetSearchResult, len(results))
	for i, r := range results {
		searchResults[i] = domain.ConsumetSearchResult{
			ID:       r.ID,
			Title:    r.Title,
			Image:    r.Image,
			Type:     r.Type,
			SubOrDub: r.SubOrDub,
		}
	}

	return searchResults, nil
}

// findConsumetID finds the Consumet ID for an anime by searching name variants in parallel
func (s *CatalogService) findConsumetID(ctx context.Context, anime *domain.Anime) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check cache for Consumet ID mapping
	cacheKey := fmt.Sprintf("consumet:mapping:%s", anime.ID)
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

	if len(searchNames) == 0 {
		return "", fmt.Errorf("anime not found on Consumet")
	}

	// Launch all name-variant searches in parallel
	type searchResult struct {
		id    string
		exact bool // true if matched by title, false if first-result fallback
	}
	exactCh := make(chan searchResult, 1)
	fallbackCh := make(chan searchResult, 1)
	var wg sync.WaitGroup

	for _, name := range searchNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			results, err := s.consumetClient.Search(name)
			if err != nil || len(results) == 0 {
				return
			}
			// Check for exact match first
			for _, r := range results {
				if matchesConsumetAnime(r.Title, anime) {
					select {
					case exactCh <- searchResult{id: r.ID, exact: true}:
					default:
					}
					return
				}
			}
			// Fallback: first result
			select {
			case fallbackCh <- searchResult{id: results[0].ID}:
			default:
			}
		}(name)
	}

	// Close channels when all goroutines finish
	go func() {
		wg.Wait()
		close(exactCh)
		close(fallbackCh)
	}()

	// Prefer exact match
	if found, ok := <-exactCh; ok {
		_ = s.cache.Set(ctx, cacheKey, found.id, 24*time.Hour)
		return found.id, nil
	}

	// Fall back to first result from any search
	if found, ok := <-fallbackCh; ok {
		_ = s.cache.Set(ctx, cacheKey, found.id, 24*time.Hour)
		return found.id, nil
	}

	return "", fmt.Errorf("anime not found on Consumet")
}

// matchesConsumetAnime checks if a search result matches an anime
func matchesConsumetAnime(resultTitle string, anime *domain.Anime) bool {
	resultLower := normalizeTitle(resultTitle)

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

// ============================================================================
// AnimeLib API Methods
// ============================================================================

// GetAnimeLibEpisodes gets episodes from AnimeLib for an anime
func (s *CatalogService) GetAnimeLibEpisodes(ctx context.Context, animeID string) (_ []domain.AnimeLibEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_episodes", start, &retErr)
	// Get anime to search by name
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("animelib:episodes:%s", animeID)
	var cached []domain.AnimeLibEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Find anime on AnimeLib
	animelibID, err := s.findAnimeLibID(ctx, anime)
	if err != nil {
		s.log.Warnw("failed to find anime on animelib",
			"anime_id", animeID,
			"name", anime.Name,
			"error", err)
		return []domain.AnimeLibEpisode{}, nil
	}

	// Get episodes
	episodes, err := s.animelibClient.GetEpisodes(animelibID)
	if err != nil {
		s.log.Warnw("failed to get animelib episodes",
			"anime_id", animeID,
			"animelib_id", animelibID,
			"error", err)
		return []domain.AnimeLibEpisode{}, nil
	}

	result := make([]domain.AnimeLibEpisode, len(episodes))
	for i, ep := range episodes {
		result[i] = domain.AnimeLibEpisode{
			ID:     ep.ID,
			Number: ep.Number,
			Name:   ep.Name,
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetAnimeLibTranslations gets available translations for an episode from AnimeLib
func (s *CatalogService) GetAnimeLibTranslations(ctx context.Context, animeID string, episodeID int) (_ []domain.AnimeLibTranslation, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_translations", start, &retErr)
	// Check cache first
	cacheKey := fmt.Sprintf("animelib:translations:%s:%d", animeID, episodeID)
	var cached []domain.AnimeLibTranslation
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Get episode streams (includes player/translation data)
	detail, err := s.animelibClient.GetEpisodeStreams(episodeID)
	if err != nil {
		s.log.Warnw("failed to get animelib episode streams",
			"anime_id", animeID,
			"episode_id", episodeID,
			"error", err)
		return []domain.AnimeLibTranslation{}, nil
	}

	// Collect unique translation teams from Players.
	// Prefer "Animelib" player (direct video) over "Kodik" (iframe) for same team.
	type teamEntry struct {
		translation domain.AnimeLibTranslation
		playerID    int // the actual player entry ID (for stream lookup)
	}
	teamMap := make(map[int]*teamEntry)

	for _, p := range detail.Players {
		// Map translation type label to internal type
		translationType := "voice"
		if p.TranslationType.ID == 1 || strings.Contains(strings.ToLower(p.TranslationType.Label), "субтитр") {
			translationType = "subtitles"
		}

		existing, exists := teamMap[p.Team.ID]
		// Prefer "Animelib" player over "Kodik" for same team
		if !exists || (p.Player == "Animelib" && existing.translation.Player != "Animelib") {
			teamMap[p.Team.ID] = &teamEntry{
				translation: domain.AnimeLibTranslation{
					ID:           p.ID, // use the player entry ID (not team ID) for stream lookup
					TeamName:     p.Team.Name,
					Type:         translationType,
					Player:       p.Player,
					HasSubtitles: len(p.Subtitles) > 0,
				},
				playerID: p.ID,
			}
		}
	}

	var result []domain.AnimeLibTranslation
	for _, entry := range teamMap {
		result = append(result, entry.translation)
	}

	if result == nil {
		result = []domain.AnimeLibTranslation{}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetAnimeLibStream gets the stream URL from AnimeLib for a specific episode and translation
func (s *CatalogService) GetAnimeLibStream(ctx context.Context, animeID string, episodeID int, translationID int) (_ *domain.AnimeLibStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("animelib").Inc()
	// Check cache first
	cacheKey := fmt.Sprintf("animelib:stream:%s:%d:%d", animeID, episodeID, translationID)
	var cached domain.AnimeLibStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	// Get episode streams
	detail, err := s.animelibClient.GetEpisodeStreams(episodeID)
	if err != nil {
		s.log.Warnw("failed to get animelib episode streams",
			"anime_id", animeID,
			"episode_id", episodeID,
			"error", err)
		return nil, errors.NotFound("stream not available on animelib")
	}

	// Find the PlayerData matching the translationID (player entry ID)
	var matched *animelib.PlayerData
	for i := range detail.Players {
		if detail.Players[i].ID == translationID {
			matched = &detail.Players[i]
			break
		}
	}

	if matched == nil {
		return nil, errors.NotFound("translation not found on animelib")
	}

	var result *domain.AnimeLibStream

	if matched.Player == "Animelib" && matched.Video != nil && len(matched.Video.Quality) > 0 {
		// Direct video: build MP4 URLs from quality array
		sources := make([]domain.AnimeLibSource, len(matched.Video.Quality))
		for i, q := range matched.Video.Quality {
			sources[i] = domain.AnimeLibSource{
				URL:     animelib.BuildVideoURL(q.Href),
				Quality: q.Quality,
			}
		}
		result = &domain.AnimeLibStream{
			Sources: sources,
		}

		// Add external subtitle files if available
		if len(matched.Subtitles) > 0 {
			subs := make([]domain.AnimeLibSubtitle, len(matched.Subtitles))
			for i, s := range matched.Subtitles {
				subs[i] = domain.AnimeLibSubtitle{
					Format: s.Format,
					URL:    s.Src,
				}
			}
			result.Subtitles = subs
		}
	} else if matched.Src != "" {
		// Kodik iframe fallback
		metrics.ParserFallbackTotal.WithLabelValues("animelib", "kodik").Inc()
		iframeURL := matched.Src
		if strings.HasPrefix(iframeURL, "//") {
			iframeURL = "https:" + iframeURL
		}
		result = &domain.AnimeLibStream{
			IframeURL: iframeURL,
		}
	} else {
		return nil, errors.NotFound("no video source available on animelib")
	}

	// Cache for 30 minutes (stream URLs may expire)
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)

	return result, nil
}

// SearchAnimeLib searches for anime on AnimeLib by title
func (s *CatalogService) SearchAnimeLib(ctx context.Context, title string) ([]domain.AnimeLibSearchResult, error) {
	results, err := s.animelibClient.Search(title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.AnimeLibSearchResult, len(results))
	for i, r := range results {
		var poster string
		if r.Cover != nil {
			poster = r.Cover.Default
		}
		searchResults[i] = domain.AnimeLibSearchResult{
			ID:      r.ID,
			Name:    r.Name,
			RusName: r.RusName,
			Poster:  poster,
			SlugURL: r.SlugURL,
		}
	}

	return searchResults, nil
}

// findAnimeLibID finds the AnimeLib integer ID for an anime by searching name variants in parallel
func (s *CatalogService) findAnimeLibID(ctx context.Context, anime *domain.Anime) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check cache for AnimeLib ID mapping
	cacheKey := fmt.Sprintf("animelib:mapping:%s", anime.ID)
	var cachedID int
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil && cachedID != 0 {
		return cachedID, nil
	}

	// Search by different name variants — prefer NameRU since AnimeLib is Russian
	searchNames := []string{}
	if anime.NameRU != "" {
		searchNames = append(searchNames, anime.NameRU)
	}
	if anime.Name != "" && anime.Name != anime.NameRU {
		searchNames = append(searchNames, anime.Name)
	}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	if len(searchNames) == 0 {
		return 0, fmt.Errorf("anime not found on AnimeLib")
	}

	// Launch all name-variant searches in parallel
	type searchResult struct{ id int }
	ch := make(chan searchResult, 1)
	var wg sync.WaitGroup

	for _, name := range searchNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			results, err := s.animelibClient.Search(name)
			if err != nil {
				s.log.Debugw("animelib search failed for variant", "name", name, "error", err)
				return
			}
			for _, r := range results {
				if matchesAnimeLibResult(r, anime) {
					select {
					case ch <- searchResult{id: r.ID}:
					default:
					}
					return
				}
			}
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	if found, ok := <-ch; ok {
		_ = s.cache.Set(ctx, cacheKey, found.id, 24*time.Hour)
		return found.id, nil
	}

	return 0, fmt.Errorf("anime not found on AnimeLib")
}

// matchesAnimeLibResult checks if an AnimeLib search result matches an anime
func matchesAnimeLibResult(result animelib.SearchResult, anime *domain.Anime) bool {
	// Check against all the AnimeLib result names
	resultNames := []string{result.Name, result.RusName, result.EngName}

	for _, resultName := range resultNames {
		if resultName == "" {
			continue
		}
		if matchesAnime(resultName, anime) {
			return true
		}
	}
	return false
}

// resolveAniListID tries to resolve the AniList ID for an anime using ARM.
// It tries ShikimoriID first (source of truth), then MALID as fallback.
// If resolved, persists the AniList ID to the database and caches it.
func (s *CatalogService) resolveAniListID(ctx context.Context, anime *domain.Anime) string {
	// Check cache first
	cacheKey := fmt.Sprintf("idmap:anilist:%s", anime.ID)
	var cachedID string
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil && cachedID != "" {
		return cachedID
	}

	var anilistID string

	// Try Shikimori ID first (source of truth)
	if anime.ShikimoriID != "" {
		armStart := time.Now()
		result, err := s.idMappingClient.ResolveByShikimoriID(anime.ShikimoriID)
		metrics.ExternalAPIDuration.WithLabelValues("arm").Observe(time.Since(armStart).Seconds())
		if err != nil {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "error").Inc()
			s.log.Warnw("ARM lookup by shikimori_id failed", "shikimori_id", anime.ShikimoriID, "error", err)
		} else {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "success").Inc()
			if result != nil && result.AniList != nil {
				anilistID = strconv.Itoa(*result.AniList)
			}
		}
	}

	// Fallback to MAL ID
	if anilistID == "" && anime.MALID != "" {
		armStart := time.Now()
		result, err := s.idMappingClient.ResolveByMALID(anime.MALID)
		metrics.ExternalAPIDuration.WithLabelValues("arm").Observe(time.Since(armStart).Seconds())
		if err != nil {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "error").Inc()
			s.log.Warnw("ARM lookup by mal_id failed", "mal_id", anime.MALID, "error", err)
		} else {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "success").Inc()
			if result != nil && result.AniList != nil {
				anilistID = strconv.Itoa(*result.AniList)
			}
		}
	}

	if anilistID == "" {
		return ""
	}

	// Persist to database
	if err := s.animeRepo.UpdateAniListID(ctx, anime.ID, anilistID); err != nil {
		s.log.Warnw("failed to persist resolved AniList ID", "anime_id", anime.ID, "anilist_id", anilistID, "error", err)
	} else {
		s.log.Infow("resolved and saved AniList ID via ARM", "anime_id", anime.ID, "anilist_id", anilistID)
	}

	// Cache for 24 hours
	_ = s.cache.Set(ctx, cacheKey, anilistID, 24*time.Hour)

	return anilistID
}

// GetJimakuSubtitles fetches Japanese subtitles from Jimaku for an anime episode
func (s *CatalogService) GetJimakuSubtitles(ctx context.Context, animeID string, episode int) (*domain.JimakuSubtitleResponse, error) {
	if !s.jimakuClient.IsConfigured() {
		return nil, errors.NotFound("jimaku subtitles not configured")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("jimaku:subs:%s:%d", animeID, episode)
	var cached domain.JimakuSubtitleResponse
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	// Look up the anime to get AniList ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, errors.NotFound("anime not found")
	}

	// Resolve AniList ID if missing, using ARM
	if anime.AniListID == "" {
		resolved := s.resolveAniListID(ctx, anime)
		if resolved == "" {
			return nil, errors.NotFound("no AniList ID available for this anime")
		}
		anime.AniListID = resolved
	}

	anilistID, err := strconv.Atoi(anime.AniListID)
	if err != nil {
		return nil, errors.NotFound("invalid AniList ID format")
	}

	// Query Jimaku API
	jimakuStart := time.Now()
	files, entryName, err := s.jimakuClient.GetSubtitlesForEpisode(anilistID, episode)
	metrics.ExternalAPIDuration.WithLabelValues("jimaku").Observe(time.Since(jimakuStart).Seconds())
	if err != nil {
		metrics.ExternalAPIRequestsTotal.WithLabelValues("jimaku", "error").Inc()
		metrics.SubtitleRequestsTotal.WithLabelValues("jimaku", "error").Inc()
		s.log.Warnw("failed to get jimaku subtitles",
			"anime_id", animeID,
			"anilist_id", anilistID,
			"episode", episode,
			"error", err)
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to fetch jimaku subtitles")
	}
	metrics.ExternalAPIRequestsTotal.WithLabelValues("jimaku", "success").Inc()
	metrics.SubtitleRequestsTotal.WithLabelValues("jimaku", "success").Inc()

	// Map to domain types
	subtitles := make([]domain.JimakuSubtitle, 0, len(files))
	for _, f := range files {
		subtitles = append(subtitles, domain.JimakuSubtitle{
			URL:      f.URL,
			FileName: f.Name,
			Lang:     "Japanese",
			Format:   jimaku.FileFormat(f.Name),
		})
	}

	result := &domain.JimakuSubtitleResponse{
		Subtitles: subtitles,
		EntryName: entryName,
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}
