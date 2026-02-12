package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

type CatalogHandler struct {
	catalogService *service.CatalogService
	log            *logger.Logger
}

func NewCatalogHandler(catalogService *service.CatalogService, log *logger.Logger) *CatalogHandler {
	return &CatalogHandler{
		catalogService: catalogService,
		log:            log,
	}
}

// SearchAnime handles anime search requests
func (h *CatalogHandler) SearchAnime(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		httputil.BadRequest(w, "search query must be at least 2 characters")
		return
	}

	filters := h.parseFilters(r)
	filters.Query = query

	animes, total, err := h.catalogService.SearchAnime(r.Context(), filters)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       filters.Page,
		PageSize:   filters.PageSize,
		TotalCount: total,
		TotalPages: int((total + int64(filters.PageSize) - 1) / int64(filters.PageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// BrowseAnime handles anime browsing requests
func (h *CatalogHandler) BrowseAnime(w http.ResponseWriter, r *http.Request) {
	filters := h.parseFilters(r)

	animes, total, err := h.catalogService.SearchAnime(r.Context(), filters)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       filters.Page,
		PageSize:   filters.PageSize,
		TotalCount: total,
		TotalPages: int((total + int64(filters.PageSize) - 1) / int64(filters.PageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetTrendingAnime handles getting trending anime
func (h *CatalogHandler) GetTrendingAnime(w http.ResponseWriter, r *http.Request) {
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	pageSize := pagination.ParseIntParam(r.URL.Query().Get("page_size"), 20)

	animes, total, err := h.catalogService.GetTrendingAnime(r.Context(), page, pageSize)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetPopularAnime handles getting popular anime
func (h *CatalogHandler) GetPopularAnime(w http.ResponseWriter, r *http.Request) {
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	pageSize := pagination.ParseIntParam(r.URL.Query().Get("page_size"), 20)

	animes, total, err := h.catalogService.GetPopularAnime(r.Context(), page, pageSize)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetRecentAnime handles getting recently added anime
func (h *CatalogHandler) GetRecentAnime(w http.ResponseWriter, r *http.Request) {
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	pageSize := pagination.ParseIntParam(r.URL.Query().Get("page_size"), 20)

	animes, total, err := h.catalogService.GetRecentAnime(r.Context(), page, pageSize)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetAnime handles getting a single anime
func (h *CatalogHandler) GetAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	// Detect MAL-prefixed IDs (e.g. "mal_36726") and resolve via Jikan + Shikimori.
	// Returns the anime directly if resolved, or 404 if ambiguous/not found.
	// The dedicated /mal/{malId} endpoint returns the full MALResolveResult for the frontend.
	if strings.HasPrefix(animeID, "mal_") {
		malID := strings.TrimPrefix(animeID, "mal_")
		result, err := h.catalogService.ResolveMALAnime(r.Context(), malID)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		if result.Status == "resolved" && result.Anime != nil {
			httputil.OK(w, result.Anime)
			return
		}
		// Ambiguous or not found — return 404 so the frontend shows an error
		httputil.NotFound(w, "anime not found")
		return
	}

	anime, err := h.catalogService.GetAnime(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
}

// ResolveMALAnime handles MAL ID resolution via Jikan + Shikimori name matching
func (h *CatalogHandler) ResolveMALAnime(w http.ResponseWriter, r *http.Request) {
	malID := chi.URLParam(r, "malId")
	if malID == "" {
		httputil.BadRequest(w, "MAL ID is required")
		return
	}

	result, err := h.catalogService.ResolveMALAnime(r.Context(), malID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, result)
}

// RefreshAnime refreshes anime data from Shikimori
func (h *CatalogHandler) RefreshAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	anime, err := h.catalogService.RefreshAnimeFromShikimori(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
}

// GetSchedule handles getting anime release schedule
func (h *CatalogHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	animes, err := h.catalogService.GetSchedule(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, animes)
}

// GetOngoingAnime handles getting all ongoing anime
func (h *CatalogHandler) GetOngoingAnime(w http.ResponseWriter, r *http.Request) {
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	pageSize := pagination.ParseIntParam(r.URL.Query().Get("page_size"), 50)

	animes, total, err := h.catalogService.GetOngoingAnime(r.Context(), page, pageSize)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetSeasonalAnime handles getting seasonal anime
func (h *CatalogHandler) GetSeasonalAnime(w http.ResponseWriter, r *http.Request) {
	yearStr := chi.URLParam(r, "year")
	season := chi.URLParam(r, "season")

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		httputil.BadRequest(w, "invalid year")
		return
	}

	if season != "winter" && season != "spring" && season != "summer" && season != "fall" {
		httputil.BadRequest(w, "invalid season")
		return
	}

	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	pageSize := pagination.ParseIntParam(r.URL.Query().Get("page_size"), 20)

	animes, total, err := h.catalogService.GetSeasonalAnime(r.Context(), year, season, page, pageSize)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	meta := httputil.Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	}

	httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}

// GetAnimeEpisodes handles getting episodes for an anime
func (h *CatalogHandler) GetAnimeEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	videos, err := h.catalogService.GetVideosForAnime(r.Context(), animeID, domain.VideoTypeEpisode)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, videos)
}

// GetGenres handles getting all genres
func (h *CatalogHandler) GetGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := h.catalogService.GetGenres(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, genres)
}

// GetAniboomTranslations gets available translations from Aniboom
func (h *CatalogHandler) GetAniboomTranslations(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	translations, err := h.catalogService.GetAniboomTranslations(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, translations)
}

// GetAniboomVideo gets video source URL from Aniboom
func (h *CatalogHandler) GetAniboomVideo(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeStr := r.URL.Query().Get("episode")
	translationID := r.URL.Query().Get("translation")

	if episodeStr == "" {
		httputil.BadRequest(w, "episode number is required")
		return
	}
	if translationID == "" {
		httputil.BadRequest(w, "translation ID is required")
		return
	}

	episode, err := strconv.Atoi(episodeStr)
	if err != nil {
		httputil.BadRequest(w, "invalid episode number")
		return
	}

	source, err := h.catalogService.GetAniboomVideoSource(r.Context(), animeID, episode, translationID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, source)
}

// GetKodikTranslations gets available translations from Kodik
func (h *CatalogHandler) GetKodikTranslations(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	translations, err := h.catalogService.GetKodikTranslations(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, translations)
}

// GetKodikVideo gets video embed link from Kodik
func (h *CatalogHandler) GetKodikVideo(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeStr := r.URL.Query().Get("episode")
	translationIDStr := r.URL.Query().Get("translation")

	if episodeStr == "" {
		httputil.BadRequest(w, "episode number is required")
		return
	}
	if translationIDStr == "" {
		httputil.BadRequest(w, "translation ID is required")
		return
	}

	episode, err := strconv.Atoi(episodeStr)
	if err != nil {
		httputil.BadRequest(w, "invalid episode number")
		return
	}

	translationID, err := strconv.Atoi(translationIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid translation ID")
		return
	}

	source, err := h.catalogService.GetKodikVideoSource(r.Context(), animeID, episode, translationID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, source)
}

// SearchKodik searches for anime on Kodik
func (h *CatalogHandler) SearchKodik(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		httputil.BadRequest(w, "search query must be at least 2 characters")
		return
	}

	results, err := h.catalogService.SearchKodik(r.Context(), query)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, results)
}

func (h *CatalogHandler) parseFilters(r *http.Request) domain.SearchFilters {
	query := r.URL.Query()

	filters := domain.SearchFilters{
		Query:    query.Get("q"),
		Season:   query.Get("season"),
		Status:   domain.AnimeStatus(query.Get("status")),
		Sort:     query.Get("sort"),
		Order:    query.Get("order"),
		Page:     pagination.ParseIntParam(query.Get("page"), 1),
		PageSize: pagination.ParseIntParam(query.Get("page_size"), 20),
		Source:   query.Get("source"),
	}

	if yearStr := query.Get("year"); yearStr != "" {
		if year, err := strconv.Atoi(yearStr); err == nil {
			filters.Year = &year
		}
	}

	if yearFrom := query.Get("year_from"); yearFrom != "" {
		if y, err := strconv.Atoi(yearFrom); err == nil {
			filters.YearFrom = &y
		}
	}

	if yearTo := query.Get("year_to"); yearTo != "" {
		if y, err := strconv.Atoi(yearTo); err == nil {
			filters.YearTo = &y
		}
	}

	if genres := query["genre"]; len(genres) > 0 {
		filters.GenreIDs = genres
	}

	// Normalize
	if filters.PageSize > 100 {
		filters.PageSize = 100
	}
	if filters.PageSize < 1 {
		filters.PageSize = 20
	}
	if filters.Page < 1 {
		filters.Page = 1
	}

	return filters
}

// GetPinnedTranslations returns pinned translations for an anime
func (h *CatalogHandler) GetPinnedTranslations(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	pinned, err := h.catalogService.GetPinnedTranslations(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, pinned)
}

// PinTranslation pins a translation for an anime (admin only)
func (h *CatalogHandler) PinTranslation(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	var req domain.PinTranslationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if req.TranslationID == 0 {
		httputil.BadRequest(w, "translation_id is required")
		return
	}

	if err := h.catalogService.PinTranslation(r.Context(), animeID, req); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"status": "pinned"})
}

// UnpinTranslation removes a pinned translation for an anime (admin only)
func (h *CatalogHandler) UnpinTranslation(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	translationID := pagination.ParseIntParam(chi.URLParam(r, "translationId"), 0)
	if translationID == 0 {
		httputil.BadRequest(w, "translation ID is required")
		return
	}

	if err := h.catalogService.UnpinTranslation(r.Context(), animeID, translationID); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"status": "unpinned"})
}

// GetHiAnimeEpisodes gets episodes from HiAnime
func (h *CatalogHandler) GetHiAnimeEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodes, err := h.catalogService.GetHiAnimeEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, episodes)
}

// GetHiAnimeServers gets available servers for an episode from HiAnime
func (h *CatalogHandler) GetHiAnimeServers(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeID := r.URL.Query().Get("episode")
	if episodeID == "" {
		httputil.BadRequest(w, "episode ID is required")
		return
	}

	servers, err := h.catalogService.GetHiAnimeServers(r.Context(), animeID, episodeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, servers)
}

// GetHiAnimeStream gets the stream URL from HiAnime
func (h *CatalogHandler) GetHiAnimeStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeID := r.URL.Query().Get("episode")
	serverID := r.URL.Query().Get("server")
	category := r.URL.Query().Get("category")

	if episodeID == "" {
		httputil.BadRequest(w, "episode ID is required")
		return
	}
	if serverID == "" {
		httputil.BadRequest(w, "server ID is required")
		return
	}
	if category == "" {
		category = "sub" // Default to sub
	}

	stream, err := h.catalogService.GetHiAnimeStream(r.Context(), animeID, episodeID, serverID, category)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}

// SearchHiAnime searches for anime on HiAnime
func (h *CatalogHandler) SearchHiAnime(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		httputil.BadRequest(w, "search query must be at least 2 characters")
		return
	}

	results, err := h.catalogService.SearchHiAnime(r.Context(), query)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, results)
}

// ============================================================================
// Consumet Handlers
// ============================================================================

// GetConsumetEpisodes gets episodes from Consumet
func (h *CatalogHandler) GetConsumetEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodes, err := h.catalogService.GetConsumetEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, episodes)
}

// GetConsumetServers gets available servers from Consumet
func (h *CatalogHandler) GetConsumetServers(w http.ResponseWriter, r *http.Request) {
	servers := h.catalogService.GetConsumetServers(r.Context())
	httputil.OK(w, servers)
}

// GetConsumetStream gets the stream URL from Consumet
func (h *CatalogHandler) GetConsumetStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeID := r.URL.Query().Get("episode")
	serverName := r.URL.Query().Get("server")

	if episodeID == "" {
		httputil.BadRequest(w, "episode ID is required")
		return
	}

	stream, err := h.catalogService.GetConsumetStream(r.Context(), animeID, episodeID, serverName)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}

// SearchConsumet searches for anime on Consumet
func (h *CatalogHandler) SearchConsumet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		httputil.BadRequest(w, "search query must be at least 2 characters")
		return
	}

	results, err := h.catalogService.SearchConsumet(r.Context(), query)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, results)
}
