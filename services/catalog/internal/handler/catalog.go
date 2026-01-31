package handler

import (
	"net/http"
	"strconv"

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

	anime, err := h.catalogService.GetAnime(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
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
