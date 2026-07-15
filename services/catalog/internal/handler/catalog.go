package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// Phase 15 plan 04 — scraper endpoints. Embedded so chi can route
	// /api/anime/{animeId}/scraper/* via catalogHandler.GetScraper*
	// alongside the existing animelib handler.
	*ScraperEndpointsHandler

	// 18anime (18+) endpoints. Embedded so chi can route
	// /api/anime/{animeId}/anime18/* via catalogHandler.GetAnime18*.
	*Anime18EndpointsHandler
}

func NewCatalogHandler(catalogService *service.CatalogService, log *logger.Logger) *CatalogHandler {
	scraperEp := &ScraperEndpointsHandler{}
	WireScraperEndpoints(scraperEp, catalogService, log)
	anime18Ep := &Anime18EndpointsHandler{}
	WireAnime18Endpoints(anime18Ep, catalogService, log)
	return &CatalogHandler{
		catalogService:          catalogService,
		log:                     log,
		ScraperEndpointsHandler: scraperEp,
		Anime18EndpointsHandler: anime18Ep,
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

	// Detect Shikimori-prefixed IDs (e.g. "shiki_21") and resolve via Shikimori API
	if strings.HasPrefix(animeID, "shiki_") {
		shikimoriID := strings.TrimPrefix(animeID, "shiki_")
		anime, err := h.catalogService.GetAnimeByShikimoriID(r.Context(), shikimoriID)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		httputil.OK(w, anime)
		return
	}

	anime, err := h.catalogService.GetAnime(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
}

// GetRelatedAnime handles fetching related anime (sequels, prequels, etc.)
func (h *CatalogHandler) GetRelatedAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	related, err := h.catalogService.GetRelatedAnime(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if related == nil {
		related = []domain.RelatedAnime{}
	}

	httputil.OK(w, related)
}

// GetSimilarAnime handles fetching similar anime (Shikimori /similar endpoint).
// Phase 13 (REC-SIG-06) — public read; consumed by the player service's S6
// pin cascade. Mirrors GetRelatedAnime contract (nil → empty array).
func (h *CatalogHandler) GetSimilarAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	similar, err := h.catalogService.GetSimilarAnime(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if similar == nil {
		similar = []domain.SimilarAnime{}
	}

	httputil.OK(w, similar)
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

// ResolveShikimoriAnime resolves a Shikimori ID to a local anime (fetching from Shikimori if needed)
func (h *CatalogHandler) ResolveShikimoriAnime(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimoriId")
	if shikimoriID == "" {
		httputil.BadRequest(w, "Shikimori ID is required")
		return
	}

	anime, err := h.catalogService.GetAnimeByShikimoriID(r.Context(), shikimoriID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
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

// BatchRefreshAnime refreshes stale anime metadata by status using batch Shikimori queries.
func (h *CatalogHandler) BatchRefreshAnime(w http.ResponseWriter, r *http.Request) {
	statusParam := r.URL.Query().Get("status")
	if statusParam == "" {
		httputil.BadRequest(w, "status query parameter is required (ongoing, announced, released)")
		return
	}

	status := domain.AnimeStatus(statusParam)
	if status != domain.StatusOngoing && status != domain.StatusAnnounced && status != domain.StatusReleased {
		httputil.BadRequest(w, "status must be one of: ongoing, announced, released")
		return
	}

	// Default staleness: ongoing=12h, announced=72h, released=168h
	staleHours := 168
	switch status {
	case domain.StatusOngoing:
		staleHours = 12
	case domain.StatusAnnounced:
		staleHours = 72
	}

	if override := r.URL.Query().Get("stale_hours"); override != "" {
		if h, err := strconv.Atoi(override); err == nil && h > 0 {
			staleHours = h
		}
	}

	staleBefore := time.Now().Add(-time.Duration(staleHours) * time.Hour)

	refreshed, failed, err := h.catalogService.BatchRefreshAnime(r.Context(), status, staleBefore)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"refreshed": refreshed,
		"failed":    failed,
		"total":     refreshed + failed,
		"status":    statusParam,
	})
}

// SyncCalendar triggers a calendar sync from Shikimori (called by scheduler)
func (h *CatalogHandler) SyncCalendar(w http.ResponseWriter, r *http.Request) {
	imported, updated, failed, err := h.catalogService.SyncCalendar(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"imported": imported,
		"updated":  updated,
		"failed":   failed,
		"total":    imported + updated + failed,
	})
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
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	recentOnly := r.URL.Query().Get("recent") == "true"

	animes, total, err := h.catalogService.GetOngoingAnime(r.Context(), page, pageSize, sort, order, recentOnly)
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

// GetStudios handles getting all studios that have anime (browse filter options)
func (h *CatalogHandler) GetStudios(w http.ResponseWriter, r *http.Request) {
	studios, err := h.catalogService.GetStudios(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, studios)
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

// GetKodikStream returns the decoded ad-free HLS stream for a Kodik episode.
func (h *CatalogHandler) GetKodikStream(w http.ResponseWriter, r *http.Request) {
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
	quality := 0
	if q := r.URL.Query().Get("quality"); q != "" {
		quality, _ = strconv.Atoi(q) // optional; 0 = default
	}

	source, err := h.catalogService.GetKodikStreamSource(r.Context(), animeID, episode, translationID, quality)
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

	// Phase 15 (UX-31) — Kinds filter (comma-separated, e.g. "tv,movie").
	// Whitelist matches the canonical Shikimori kind set and the frontend
	// FilterCheckboxList options. Unknown values silently drop so a malicious
	// value can't reach the SQL. Duplicates collapse via `seen`.
	if kinds := query.Get("kind"); kinds != "" {
		seen := map[string]bool{}
		for _, k := range strings.Split(kinds, ",") {
			k = strings.TrimSpace(strings.ToLower(k))
			switch k {
			case "tv", "movie", "ova", "ona", "special", "tv_special", "music", "cm", "pv":
				if !seen[k] {
					filters.Kinds = append(filters.Kinds, k)
					seen[k] = true
				}
			}
		}
	}

	// Phase 15 (UX-31) — Providers filter (comma-separated, e.g.
	// "kodik,animelib"). Whitelisted at the handler — repo treats unknown
	// keys as drop-silent, but we still strip them here so the SQL
	// builder only sees vetted values. Duplicates collapse via `seen`.
	if providers := query.Get("providers"); providers != "" {
		raw := strings.Split(providers, ",")
		seen := map[string]bool{}
		for _, p := range raw {
			p = strings.TrimSpace(strings.ToLower(p))
			switch p {
			case "kodik", "dub", "ae":
				if !seen[p] {
					filters.Providers = append(filters.Providers, p)
					seen[p] = true
				}
			}
		}
	}

	// Studios — same comma-joined OR multi-value forms as genres (?studio=id1,id2).
	if studios := query["studio"]; len(studios) > 0 {
		for _, s := range studios {
			for _, id := range strings.Split(s, ",") {
				if id = strings.TrimSpace(id); id != "" {
					filters.StudioIDs = append(filters.StudioIDs, id)
				}
			}
		}
	}

	// Frontend sends genres as a single comma-joined query param (?genre=id1,id2).
	// Handle both that form and the multi-value form (?genre=id1&genre=id2).
	if genres := query["genre"]; len(genres) > 0 {
		for _, g := range genres {
			for _, id := range strings.Split(g, ",") {
				if id = strings.TrimSpace(id); id != "" {
					filters.GenreIDs = append(filters.GenreIDs, id)
				}
			}
		}
	}

	if scoreMinStr := query.Get("score_min"); scoreMinStr != "" {
		if v, err := strconv.ParseFloat(scoreMinStr, 64); err == nil && v > 0 && v <= 10 {
			filters.ScoreMin = &v
		}
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

// ============================================================================
// Hanime Handlers
// ============================================================================

// GetHanimeEpisodes gets episodes from Hanime for an anime.
func (h *CatalogHandler) GetHanimeEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodes, err := h.catalogService.GetHanimeEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, episodes)
}

// GetHanimeStream gets stream URLs for a Hanime episode.
func (h *CatalogHandler) GetHanimeStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	slug := r.URL.Query().Get("slug")
	if slug == "" {
		httputil.BadRequest(w, "slug is required")
		return
	}

	stream, err := h.catalogService.GetHanimeStream(r.Context(), animeID, slug)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}

// ============================================================================
// AnimeJoy Handlers (RU-sub: Sibnet + AllVideo legs, two independent providers)
// ============================================================================

// GetAnimejoySibnetStream resolves the AnimeJoy *Sibnet* leg for an episode.
// Thin: read animeId (chi param) + the same `episode` query param the AnimeLib
// stream route uses + optional `team`; delegate to GetAnimejoyStream with the
// fixed "sibnet" leg.
func (h *CatalogHandler) GetAnimejoySibnetStream(w http.ResponseWriter, r *http.Request) {
	h.getAnimejoyStream(w, r, "sibnet")
}

// GetAnimejoyAllVideoStream resolves the AnimeJoy *AllVideo* leg for an episode.
// Mirror of GetAnimejoySibnetStream with the fixed "allvideo" leg. No
// intra-provider failover — the other chip is the user's fallback.
func (h *CatalogHandler) GetAnimejoyAllVideoStream(w http.ResponseWriter, r *http.Request) {
	h.getAnimejoyStream(w, r, "allvideo")
}

// getAnimejoyStream is the shared body for both leg handlers.
func (h *CatalogHandler) getAnimejoyStream(w http.ResponseWriter, r *http.Request, leg string) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeStr := r.URL.Query().Get("episode")
	if episodeStr == "" {
		httputil.BadRequest(w, "episode is required")
		return
	}
	episode, err := strconv.Atoi(episodeStr)
	if err != nil {
		httputil.BadRequest(w, "invalid episode")
		return
	}

	teamID := r.URL.Query().Get("team")

	stream, err := h.catalogService.GetAnimejoyStream(r.Context(), animeID, leg, episode, teamID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}

// GetAnimejoySibnetEpisodes returns the AnimeJoy *Sibnet* leg's episode + team
// inventory for a title — the per-provider episode list the FE adapter's
// listEpisodes / listTeams read. Thin: read animeId (chi param); delegate to
// GetAnimejoyLegInfo with the fixed "sibnet" leg.
func (h *CatalogHandler) GetAnimejoySibnetEpisodes(w http.ResponseWriter, r *http.Request) {
	h.getAnimejoyEpisodes(w, r, "sibnet")
}

// GetAnimejoyAllVideoEpisodes is the AllVideo mirror of GetAnimejoySibnetEpisodes.
func (h *CatalogHandler) GetAnimejoyAllVideoEpisodes(w http.ResponseWriter, r *http.Request) {
	h.getAnimejoyEpisodes(w, r, "allvideo")
}

// getAnimejoyEpisodes is the shared body for both leg episode handlers. Mirrors
// getAnimejoyStream: animeId (chi) only — no episode/team query params, since the
// FE needs the whole per-leg list to build its picker.
func (h *CatalogHandler) getAnimejoyEpisodes(w http.ResponseWriter, r *http.Request, leg string) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	info, err := h.catalogService.GetAnimejoyLegInfo(r.Context(), animeID, leg)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, info)
}

// GetJimakuSubtitles fetches Japanese subtitles from Jimaku for an anime episode
func (h *CatalogHandler) GetJimakuSubtitles(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeStr := r.URL.Query().Get("episode")
	if episodeStr == "" {
		httputil.BadRequest(w, "episode number is required")
		return
	}

	episode, err := strconv.Atoi(episodeStr)
	if err != nil || episode < 1 {
		httputil.BadRequest(w, "episode must be a positive number")
		return
	}

	result, err := h.catalogService.GetJimakuSubtitles(r.Context(), animeID, episode)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, result)
}
