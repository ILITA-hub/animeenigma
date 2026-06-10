package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// reviewResponse is the EXACT wire shape every review endpoint returns. It
// has 9 JSON-tagged scalar fields plus the optional `anime` preload — and
// nothing else, even though the underlying `*domain.AnimeListEntry` carries
// additional fields (notes, tags, mal_id, is_rewatching, priority,
// started_at, completed_at, updated_at) that MUST NOT leak. SOCIAL-NF-01
// contract — 2026-05-21 update promoted `status` + `episodes` from forbidden
// to allowed as part of the Steam-style review-context feature (see
// docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md).
// Tests in review_shape_test.go assert this projection is in place on every
// method.
type reviewResponse struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	AnimeID    string    `json:"anime_id"`
	Username   string    `json:"username"`
	Score      int       `json:"score"`
	ReviewText string    `json:"review_text"`
	CreatedAt  time.Time `json:"created_at"`
	// Status and Episodes — Steam-style review context (2026-05-21). Live
	// values from anime_list.status, NOT snapshotted. If the reviewer keeps
	// watching after publishing, these numbers update.
	//
	// Episodes is max(anime_list.episodes, distinct completed episodes in
	// watch_progress) — so a passive watcher who never updated their list
	// doesn't show a false ⚠️ "0 episodes" (repo-todo 19:00:02).
	// IsRewatching surfaces the "🔁 On rewatch" segment on review cards
	// (repo-todo 19:00:01). Spec:
	// docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md.
	Status       string            `json:"status"`
	Episodes     int               `json:"episodes"`
	IsRewatching bool              `json:"is_rewatching"`
	Anime        *domain.AnimeInfo `json:"anime,omitempty"`
	// Reactions — per-emoji aggregate counts (with reacted_by_me for the
	// requesting viewer). MyReactions is the convenience subset of emojis the
	// viewer has reacted with, consumed by the frontend's `viewer-reacted`
	// prop. Both always present (never null) so the UI can bind safely.
	// AUTO-408.
	Reactions   []domain.ReactionCount `json:"reactions"`
	MyReactions []string               `json:"my_reactions"`
}

// toReviewResponse projects an AnimeListEntry into the wire-stable
// reviewResponse shape. Used by every review endpoint that returns JSON.
func toReviewResponse(e *domain.AnimeListEntry) reviewResponse {
	// Normalize reactions to a non-nil slice so JSON encodes `[]` not `null`,
	// and derive the viewer's reacted-emoji subset for the `my_reactions` field.
	reactions := e.Reactions
	if reactions == nil {
		reactions = []domain.ReactionCount{}
	}
	myReactions := make([]string, 0, len(reactions))
	for _, rc := range reactions {
		if rc.ReactedByMe {
			myReactions = append(myReactions, rc.Emoji)
		}
	}
	return reviewResponse{
		ID:          e.ID,
		UserID:      e.UserID,
		AnimeID:     e.AnimeID,
		Username:    e.Username,
		Score:       e.Score,
		ReviewText:  e.ReviewText,
		CreatedAt:   e.CreatedAt,
		Status:       e.Status,
		Episodes:     e.Episodes,
		IsRewatching: e.IsRewatching,
		Anime:        e.Anime,
		Reactions:    reactions,
		MyReactions:  myReactions,
	}
}

// toReviewResponses projects a slice of entries; returns a non-nil empty
// slice when input is nil so JSON encodes as `[]` not `null` (matches the
// pre-refactor behavior — frontend never sees `null` for list endpoints).
func toReviewResponses(entries []*domain.AnimeListEntry) []reviewResponse {
	out := make([]reviewResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toReviewResponse(e))
	}
	return out
}

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

	entry, err := h.reviewService.CreateOrUpdateReview(r.Context(), claims.UserID, claims.Username, claims.Role == authz.RoleAdmin, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, toReviewResponse(entry))
}

// GetBatchAnimeRatings returns average ratings for multiple anime
func (h *ReviewHandler) GetBatchAnimeRatings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AnimeIDs []string `json:"anime_ids"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if len(req.AnimeIDs) == 0 {
		httputil.OK(w, map[string]interface{}{"ratings": map[string]interface{}{}})
		return
	}
	if len(req.AnimeIDs) > 100 {
		httputil.BadRequest(w, "maximum 100 anime IDs per request")
		return
	}

	ratings, err := h.reviewService.GetBatchAnimeRatings(r.Context(), req.AnimeIDs)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{"ratings": ratings})
}

// GetAnimeReviews returns all reviews for an anime
func (h *ReviewHandler) GetAnimeReviews(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	// Optional auth: when a valid JWT rode along (player's OptionalAuthMiddleware
	// decoded it), resolve the viewer so each reaction carries reacted_by_me.
	// Anonymous callers get viewerID=nil → all reacted_by_me=false. AUTO-408.
	var viewerID *string
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		uid := claims.UserID
		viewerID = &uid
	}

	entries, err := h.reviewService.GetAnimeReviews(r.Context(), animeID, viewerID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, toReviewResponses(entries))
}

// ReactToReview toggles the calling user's emoji reaction on a review.
// POST /api/anime/{animeId}/reviews/{reviewId}/reactions/{emoji} — auth
// required. Returns { added: bool, counts: []ReactionCount } where `added`
// reports whether the reaction was added (true) or removed (false). AUTO-408.
func (h *ReviewHandler) ReactToReview(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	reviewID := chi.URLParam(r, "reviewId")
	emoji := chi.URLParam(r, "emoji")
	if animeID == "" || reviewID == "" || emoji == "" {
		httputil.BadRequest(w, "anime_id, review_id and emoji are required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	added, counts, err := h.reviewService.ToggleReaction(r.Context(), animeID, reviewID, claims.UserID, claims.Username, emoji)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if counts == nil {
		counts = []domain.ReactionCount{}
	}

	httputil.OK(w, map[string]interface{}{"added": added, "counts": counts})
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

	entry, err := h.reviewService.GetUserReview(r.Context(), claims.UserID, animeID)
	if err != nil {
		// Return null if not found (preserves the pre-refactor behavior —
		// frontend treats null as "user has no review yet").
		httputil.OK(w, nil)
		return
	}

	httputil.OK(w, toReviewResponse(entry))
}

// GetUserReviews returns all reviews by the current user
func (h *ReviewHandler) GetUserReviews(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entries, err := h.reviewService.GetUserReviews(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, toReviewResponses(entries))
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
