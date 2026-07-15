package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// SearchAnime searches for anime, fetching from Shikimori if not found locally
func (s *CatalogService) SearchAnime(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	// If source=shikimori, force search on Shikimori (skip cache)
	if filters.Source == "shikimori" && filters.Query != "" {
		metrics.SearchRequestsTotal.WithLabelValues("shikimori").Inc()
		return s.searchShikimori(ctx, filters)
	}

	// Check search result cache for query searches. The key reflects the FULL
	// canonical filter set (filters.CacheKey) — keying on query+page alone made
	// two requests with the same query but different genre/year/kind/sort/score
	// filters collide in the 15m search cache and serve stale, wrong results.
	var searchCacheKey string
	if filters.Query != "" {
		searchCacheKey = filters.CacheKey()
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
		s.enrichAll(ctx, animes)
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
	s.enrichAll(ctx, shikimoriAnimes)

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

	// Cache the result. Ongoing anime change often (episodes_aired /
	// next_episode_at advance as episodes air), so cache their detail row
	// briefly so airing data self-heals even if an invalidation is missed;
	// released/finished rows are stable, keep the long TTL.
	ttl := cache.TTLAnimeDetails
	if dbAnime.Status == domain.StatusOngoing {
		ttl = cache.TTLOngoingAnimeDetails
	}
	_ = s.cache.Set(ctx, cacheKey, dbAnime, ttl)

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

// GetRelatedAnime fetches related anime (sequels, prequels, etc.) for the given anime.
func (s *CatalogService) GetRelatedAnime(ctx context.Context, animeID string) ([]domain.RelatedAnime, error) {
	anime, err := s.GetAnime(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime.ShikimoriID == "" {
		return nil, nil
	}

	cacheKey := cache.KeyRelatedAnime(anime.ShikimoriID)
	var cached []domain.RelatedAnime
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	related, err := s.shikimoriClient.GetRelatedAnime(ctx, anime.ShikimoriID)
	if err != nil {
		s.log.Warnw("failed to fetch related anime", "anime_id", animeID, "error", err)
		return nil, nil
	}

	for i := range related {
		local, err := s.animeRepo.GetByShikimoriID(ctx, related[i].ShikimoriID)
		if err == nil && local != nil {
			related[i].LocalID = local.ID
		}
	}

	_ = s.cache.Set(ctx, cacheKey, related, cache.TTLAnimeDetails)
	return related, nil
}

// GetSimilarAnime fetches similar anime from Shikimori for the given local anime.
// Phase 13 (REC-SIG-06) — used by the player service's S6 pin cascade as a
// Shikimori fallback when the local co-occurrence pool yields fewer than 5
// post-S11-filter candidates. Mirrors GetRelatedAnime: cache check → Shikimori
// fallback → enrichment with LocalID via GetByShikimoriID → cache write.
func (s *CatalogService) GetSimilarAnime(ctx context.Context, animeID string) ([]domain.SimilarAnime, error) {
	anime, err := s.GetAnime(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime.ShikimoriID == "" {
		return nil, nil
	}

	cacheKey := cache.KeySimilarAnime(anime.ShikimoriID)
	var cached []domain.SimilarAnime
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	similar, err := s.shikimoriClient.GetSimilarAnime(ctx, anime.ShikimoriID)
	if err != nil {
		s.log.Warnw("failed to fetch similar anime", "anime_id", animeID, "error", err)
		return nil, nil
	}

	for i := range similar {
		local, err := s.animeRepo.GetByShikimoriID(ctx, similar[i].ShikimoriID)
		if err == nil && local != nil {
			similar[i].LocalID = local.ID
		}
	}

	_ = s.cache.Set(ctx, cacheKey, similar, cache.TTLAnimeDetails)
	return similar, nil
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

	// Step 3+: Fall back to Jikan title matching (for IDs Shikimori doesn't recognize)
	return s.resolveMALViaJikan(ctx, malID)
}

// resolveMALViaJikan resolves a MAL ID that direct Shikimori lookup couldn't find,
// by fetching MAL metadata via Jikan, searching Shikimori by romanized title, and
// matching on exact name. Returns an "ambiguous" result (never an error) when the
// Jikan fetch, Shikimori search, or title match fails; a "resolved" result on match.
func (s *CatalogService) resolveMALViaJikan(ctx context.Context, malID string) (*domain.MALResolveResult, error) {
	s.log.Infow("resolving MAL ID via Jikan", "mal_id", malID)

	malInfo, err := s.jikanClient.GetAnimeByID(ctx, malID)
	if err != nil {
		s.log.Warnw("failed to fetch MAL info via Jikan", "mal_id", malID, "error", err)
		return &domain.MALResolveResult{
			Status: "ambiguous",
			MALID:  malID,
		}, nil
	}

	// Search Shikimori by romanized title
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

	// Look for an exact name match
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

	// Store matched anime, backfill MAL ID
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

// GetSeasonalAnime gets anime for a specific season
func (s *CatalogService) GetSeasonalAnime(ctx context.Context, year int, season string, page, pageSize int) ([]*domain.Anime, int64, error) {
	// Try local first
	animes, total, err := s.animeRepo.GetBySeason(ctx, year, season, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// If we have local results, return them
	if len(animes) > 0 {
		s.enrichAll(ctx, animes)
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

	s.enrichAll(ctx, shikimoriAnimes)

	return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
}

// GetTrendingAnime gets trending anime from cache (backed by Shikimori).
// Cache-first: returns cached top anime (24h TTL), on miss fetches from Shikimori and caches.
func (s *CatalogService) GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	// Only cache page 1 (the main top anime list)
	if page == 1 {
		var cached []*domain.Anime
		if err := s.cache.Get(ctx, cache.KeyTopAnime(), &cached); err == nil && len(cached) > 0 {
			result := sliceToPageSize(cached, pageSize)
			s.enrichAll(ctx, result)
			return result, int64(len(cached)), nil
		}
	}

	// Cache miss — always fetch 20 for the cache to avoid smaller requests poisoning it
	fetchSize := pageSize
	if page == 1 && fetchSize < 20 {
		fetchSize = 20
	}
	shikimoriAnimes, err := s.shikimoriClient.GetTrendingAnime(ctx, page, fetchSize)
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
		s.enrichAll(ctx, animes)
		return animes, total, nil
	}

	// Upsert to DB and cache
	for _, anime := range shikimoriAnimes {
		if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
			s.log.Warnw("failed to store trending anime", "error", err)
		}
	}
	s.enrichAll(ctx, shikimoriAnimes)

	// Cache page 1 results for 24 hours
	if page == 1 && len(shikimoriAnimes) > 0 {
		if err := s.cache.Set(ctx, cache.KeyTopAnime(), shikimoriAnimes, cache.TTLTopAnime); err != nil {
			s.log.Warnw("failed to cache top anime", "error", err)
		}
	}

	return sliceToPageSize(shikimoriAnimes, pageSize), int64(len(shikimoriAnimes)), nil
}

// sliceToPageSize returns the first pageSize elements of animes, or all of them
// when pageSize is not smaller than the slice length.
func sliceToPageSize(animes []*domain.Anime, pageSize int) []*domain.Anime {
	if pageSize < len(animes) {
		return animes[:pageSize]
	}
	return animes
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
		}
		s.enrichAll(ctx, shikimoriAnimes)

		return shikimoriAnimes, int64(len(shikimoriAnimes)), nil
	}

	s.enrichAll(ctx, animes)

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

	s.enrichAll(ctx, animes)

	return animes, total, nil
}

// GetSchedule gets anime release schedule (ongoing with next episode dates)
func (s *CatalogService) GetSchedule(ctx context.Context) ([]*domain.Anime, error) {
	animes, err := s.animeRepo.GetSchedule(ctx)
	if err != nil {
		return nil, err
	}

	s.enrichAll(ctx, animes)

	return animes, nil
}

// GetOngoingAnime gets all ongoing anime
func (s *CatalogService) GetOngoingAnime(ctx context.Context, page, pageSize int, sort, order string, recentOnly bool) ([]*domain.Anime, int64, error) {
	animes, total, err := s.animeRepo.GetOngoingAnime(ctx, page, pageSize, sort, order, recentOnly)
	if err != nil {
		return nil, 0, err
	}

	s.enrichAll(ctx, animes)

	return animes, total, nil
}

// GetStudios gets all studios that have anime (browse filter options).
func (s *CatalogService) GetStudios(ctx context.Context) ([]domain.Studio, error) {
	cacheKey := "studios:all"
	var studios []domain.Studio
	if err := s.cache.Get(ctx, cacheKey, &studios); err == nil {
		return studios, nil
	}

	studios, err := s.animeRepo.ListStudios(ctx)
	if err != nil {
		return nil, err
	}

	_ = s.cache.Set(ctx, cacheKey, studios, cache.TTLGenreList)
	return studios, nil
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
