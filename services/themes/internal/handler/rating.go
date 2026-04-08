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

type RatingHandler struct {
	ratingService *service.RatingService
	log           *logger.Logger
}

func NewRatingHandler(ratingService *service.RatingService, log *logger.Logger) *RatingHandler {
	return &RatingHandler{
		ratingService: ratingService,
		log:           log,
	}
}

// RateTheme handles POST /api/themes/{id}/rate
func (h *RatingHandler) RateTheme(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	themeID := chi.URLParam(r, "id")
	if themeID == "" {
		httputil.BadRequest(w, "missing theme id")
		return
	}

	var req domain.RateRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if err := h.ratingService.Rate(r.Context(), claims.UserID, themeID, req.Score); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"theme_id": themeID,
		"score":    req.Score,
	})
}

// UnrateTheme handles DELETE /api/themes/{id}/rate
func (h *RatingHandler) UnrateTheme(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	themeID := chi.URLParam(r, "id")
	if themeID == "" {
		httputil.BadRequest(w, "missing theme id")
		return
	}

	if err := h.ratingService.Unrate(r.Context(), claims.UserID, themeID); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}

// GetMyRatings handles GET /api/themes/my-ratings
func (h *RatingHandler) GetMyRatings(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	year := 0
	if yearStr := r.URL.Query().Get("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			year = y
		}
	}
	season := r.URL.Query().Get("season")

	ratings, err := h.ratingService.GetUserRatings(r.Context(), claims.UserID, year, season)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if ratings == nil {
		ratings = []domain.ThemeRating{}
	}
	httputil.OK(w, ratings)
}
