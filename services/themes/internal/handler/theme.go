package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/service"
	"github.com/go-chi/chi/v5"
)

type ThemeHandler struct {
	themeService *service.ThemeService
	log          *logger.Logger
}

func NewThemeHandler(themeService *service.ThemeService, log *logger.Logger) *ThemeHandler {
	return &ThemeHandler{
		themeService: themeService,
		log:          log,
	}
}

const (
	// defaultListLimit bounds GET /api/themes when no limit is supplied, so the
	// expensive grouped two-LEFT-JOIN scan never returns the whole table.
	defaultListLimit = 100
	// maxListLimit is the hard ceiling a client may request.
	maxListLimit = 500
)

// parseListParams extracts and clamps the GET /api/themes query parameters.
// Limit defaults to defaultListLimit when absent, non-numeric, or <= 0, and is
// capped at maxListLimit. Offset is clamped to >= 0.
func parseListParams(r *http.Request) domain.ThemeListParams {
	q := r.URL.Query()
	params := domain.ThemeListParams{
		Season: q.Get("season"),
		Type:   q.Get("type"),
		Sort:   q.Get("sort"),
		Limit:  defaultListLimit,
	}

	if yearStr := q.Get("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			params.Year = y
		}
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			if n > maxListLimit {
				n = maxListLimit
			}
			params.Limit = n
		}
	}

	if offsetStr := q.Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n > 0 {
			params.Offset = n
		}
	}

	return params
}

// ListThemes handles GET /api/themes
func (h *ThemeHandler) ListThemes(w http.ResponseWriter, r *http.Request) {
	params := parseListParams(r)

	// Optional auth — extract user ID if present
	claims, _ := authz.ClaimsFromContext(r.Context())
	if claims != nil {
		params.UserID = claims.UserID
	}

	themes, err := h.themeService.List(r.Context(), params)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if themes == nil {
		themes = []domain.AnimeTheme{}
	}
	httputil.OK(w, themes)
}

// GetTheme handles GET /api/themes/{id}
func (h *ThemeHandler) GetTheme(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "missing theme id")
		return
	}

	userID := ""
	claims, _ := authz.ClaimsFromContext(r.Context())
	if claims != nil {
		userID = claims.UserID
	}

	theme, err := h.themeService.GetByID(r.Context(), id, userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, theme)
}
