package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type ListHandler struct {
	listService *service.ListService
	log         *logger.Logger
}

func NewListHandler(listService *service.ListService, log *logger.Logger) *ListHandler {
	return &ListHandler{
		listService: listService,
		log:         log,
	}
}

// GetUserList returns user's anime list
func (h *ListHandler) GetUserList(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	status := r.URL.Query().Get("status")
	params := parsePaginationParams(r)

	entries, total, err := h.listService.GetUserListPaginated(r.Context(), claims.UserID, status, params)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if entries == nil {
		entries = []*domain.AnimeListEntry{}
	}

	totalPages := int(total) / params.PerPage
	if int(total)%params.PerPage > 0 {
		totalPages++
	}

	httputil.JSONWithMeta(w, http.StatusOK, entries, httputil.Meta{
		Page:       params.Page,
		PageSize:   params.PerPage,
		TotalCount: total,
		TotalPages: totalPages,
	})
}

// GetWatchlistStatuses returns lightweight anime_id+status pairs for the user's list
func (h *ListHandler) GetWatchlistStatuses(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	statuses, err := h.listService.GetUserStatuses(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if statuses == nil {
		statuses = []domain.AnimeStatusEntry{}
	}

	httputil.OK(w, statuses)
}

// AddToList adds an anime to user's watchlist
func (h *ListHandler) AddToList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AnimeID string `json:"anime_id"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	// Add with default status "plan_to_watch"
	listReq := &domain.UpdateListRequest{
		AnimeID: req.AnimeID,
		Status:  "plan_to_watch",
	}

	entry, err := h.listService.UpdateListEntry(r.Context(), claims.UserID, claims.Username, listReq)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	metrics.WatchlistOperationsTotal.WithLabelValues("add").Inc()
	httputil.Created(w, entry)
}

// UpdateListEntry updates an anime list entry
func (h *ListHandler) UpdateListEntry(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateListRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entry, err := h.listService.UpdateListEntry(r.Context(), claims.UserID, claims.Username, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, entry)
}

// GetUserAnimeEntry returns a single anime entry from user's list
func (h *ListHandler) GetUserAnimeEntry(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entry, err := h.listService.GetUserAnimeEntry(r.Context(), claims.UserID, animeID)
	if err != nil {
		// Return null if not found (not an error)
		httputil.OK(w, nil)
		return
	}

	httputil.OK(w, entry)
}

// DeleteListEntry removes an anime from user's list
func (h *ListHandler) DeleteListEntry(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	err := h.listService.DeleteListEntry(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	metrics.WatchlistOperationsTotal.WithLabelValues("remove").Inc()
	httputil.NoContent(w)
}

// MarkEpisodeWatched marks an episode as watched and updates the episodes count
func (h *ListHandler) MarkEpisodeWatched(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	var req struct {
		Episode int `json:"episode"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entry, err := h.listService.MarkEpisodeWatched(r.Context(), claims.UserID, animeID, req.Episode)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, entry)
}

// MigrateListEntry migrates a list entry from one anime_id to another.
// Used when resolving mal_XXXXX entries to real UUIDs.
func (h *ListHandler) MigrateListEntry(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req struct {
		OldAnimeID string `json:"old_anime_id"`
		NewAnimeID string `json:"new_anime_id"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.OldAnimeID == "" || req.NewAnimeID == "" {
		httputil.BadRequest(w, "old_anime_id and new_anime_id are required")
		return
	}

	entry, err := h.listService.MigrateListEntry(
		r.Context(), claims.UserID,
		req.OldAnimeID, req.NewAnimeID,
	)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, entry)
}

// GetPublicWatchlist returns a user's public watchlist filtered by allowed statuses
func (h *ListHandler) GetPublicWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user ID is required")
		return
	}

	var statuses []string
	if s := r.URL.Query().Get("status"); s != "" {
		statuses = []string{s}
	} else if statusesParam := r.URL.Query().Get("statuses"); statusesParam != "" {
		for _, s := range splitAndTrim(statusesParam, ",") {
			if s != "" {
				statuses = append(statuses, s)
			}
		}
	}

	params := parsePaginationParams(r)

	entries, total, err := h.listService.GetPublicWatchlistPaginated(r.Context(), userID, statuses, params)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if entries == nil {
		entries = []*domain.AnimeListEntry{}
	}

	totalPages := int(total) / params.PerPage
	if int(total)%params.PerPage > 0 {
		totalPages++
	}

	httputil.JSONWithMeta(w, http.StatusOK, entries, httputil.Meta{
		Page:       params.Page,
		PageSize:   params.PerPage,
		TotalCount: total,
		TotalPages: totalPages,
	})
}

// GetPublicWatchlistStats returns aggregate stats for a user's public watchlist
func (h *ListHandler) GetPublicWatchlistStats(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user ID is required")
		return
	}

	var statuses []string
	if s := r.URL.Query().Get("statuses"); s != "" {
		for _, s := range splitAndTrim(s, ",") {
			if s != "" {
				statuses = append(statuses, s)
			}
		}
	}

	stats, err := h.listService.GetPublicWatchlistStats(r.Context(), userID, statuses)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stats)
}

func parsePaginationParams(r *http.Request) *domain.PaginationParams {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	params := &domain.PaginationParams{
		Page:    page,
		PerPage: perPage,
		Sort:    sort,
		Order:   order,
	}
	params.Validate()
	return params
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range split(s, sep) {
		trimmed := trim(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
