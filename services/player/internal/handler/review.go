package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type ReviewHandler struct {
	reviewService *service.ReviewService
	log           *logger.Logger
}

func NewReviewHandler(reviewService *service.ReviewService, log *logger.Logger) *ReviewHandler {
	return &ReviewHandler{
		reviewService: reviewService,
		log:           log,
	}
}

// CreateOrUpdateReview creates or updates a review
func (h *ReviewHandler) CreateOrUpdateReview(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateReviewRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	review, err := h.reviewService.CreateOrUpdateReview(r.Context(), claims.UserID, claims.Username, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, review)
}

// GetAnimeReviews returns all reviews for an anime
func (h *ReviewHandler) GetAnimeReviews(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	reviews, err := h.reviewService.GetAnimeReviews(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, reviews)
}

// GetAnimeRating returns the average rating for an anime
func (h *ReviewHandler) GetAnimeRating(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	rating, err := h.reviewService.GetAnimeRating(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, rating)
}

// GetUserReview returns the current user's review for an anime
func (h *ReviewHandler) GetUserReview(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	review, err := h.reviewService.GetUserReview(r.Context(), claims.UserID, animeID)
	if err != nil {
		// Return null if not found
		httputil.OK(w, nil)
		return
	}

	httputil.OK(w, review)
}

// GetUserReviews returns all reviews by the current user
func (h *ReviewHandler) GetUserReviews(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	reviews, err := h.reviewService.GetUserReviews(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, reviews)
}

// DeleteReview deletes the current user's review
func (h *ReviewHandler) DeleteReview(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	err := h.reviewService.DeleteReview(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}
