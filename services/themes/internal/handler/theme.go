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

// ListThemes handles GET /api/themes
func (h *ThemeHandler) ListThemes(w http.ResponseWriter, r *http.Request) {
	params := domain.ThemeListParams{
		Season: r.URL.Query().Get("season"),
		Type:   r.URL.Query().Get("type"),
		Sort:   r.URL.Query().Get("sort"),
	}

	if yearStr := r.URL.Query().Get("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			params.Year = y
		}
	}

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
