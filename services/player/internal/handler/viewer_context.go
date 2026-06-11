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

// ViewerContextResponse is the aggregate payload for the anime page. It
// collapses what used to be 6-7 separate page-load round-trips (rating,
// watchers-count, progress, watchlist entry, my review, saved combo) into a
// single optional-auth request. Anonymous callers receive only the public
// fields (rating, watchers_count); the user-scoped fields stay null.
//
// Page-fetch optimization, 2026-06-11. Frontend consumer:
// frontend/web/src/views/Anime.vue via animeApi.getViewerContext.
type ViewerContextResponse struct {
	Rating        *domain.AnimeRating `json:"rating"`
	WatchersCount int64               `json:"watchers_count"`
	// User-scoped — null for anonymous callers.
	Progress       []*domain.WatchProgress     `json:"progress"`
	WatchlistEntry *domain.AnimeListEntry      `json:"watchlist_entry"`
	MyReview       *reviewResponse             `json:"my_review"`
	Combo          *domain.UserAnimePreference `json:"combo"`
}

type ViewerContextHandler struct {
	progressService *service.ProgressService
	listService     *service.ListService
	reviewService   *service.ReviewService
	prefService     *service.PreferenceService
	log             *logger.Logger
}

func NewViewerContextHandler(
	progressService *service.ProgressService,
	listService *service.ListService,
	reviewService *service.ReviewService,
	prefService *service.PreferenceService,
	log *logger.Logger,
) *ViewerContextHandler {
	return &ViewerContextHandler{
		progressService: progressService,
		listService:     listService,
		reviewService:   reviewService,
		prefService:     prefService,
		log:             log,
	}
}

// GetViewerContext returns the aggregate anime-page context for the calling
// viewer. Mounted under OptionalAuthMiddleware: a valid JWT populates the
// user-scoped fields, anonymous callers get the public subset.
//
// Error semantics mirror the individual endpoints this aggregates:
//   - rating / watchers-count failures are fatal (500) — they are public core
//     data and a failure means the DB is unhappy;
//   - user-scoped lookups treat "not found" (or any error) as null, exactly
//     like GetUserAnimeEntry / GetUserReview / GetAnimePreference do today.
func (h *ViewerContextHandler) GetViewerContext(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}

	ctx := r.Context()
	resp := ViewerContextResponse{}

	rating, err := h.reviewService.GetAnimeRating(ctx, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	resp.Rating = rating

	count, err := h.listService.GetWatchersCount(ctx, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	resp.WatchersCount = count

	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims == nil {
		httputil.OK(w, resp)
		return
	}
	userID := claims.UserID

	if progress, err := h.progressService.GetProgress(ctx, userID, animeID); err == nil {
		resp.Progress = progress
	} else {
		h.log.Warnw("viewer-context: progress lookup failed",
			"user_id", userID, "anime_id", animeID, "error", err)
	}

	// NOTE: the repo returns (nil, nil) — not an error — when no row exists.
	if entry, err := h.listService.GetUserAnimeEntry(ctx, userID, animeID); err == nil && entry != nil {
		resp.WatchlistEntry = entry
	} else {
		// Legacy MAL imports store entries under anime_id="mal_{malId}" until
		// the user first visits the anime page (which auto-migrates them).
		// Surface such an entry so the frontend can show the status AND run
		// its existing migration path — without re-fetching the full
		// statuses list. The MAL id comes from the ?mal_id= override when
		// supplied, otherwise from the catalog-owned animes row — so the
		// frontend's route-guard prefetch can fire this request before the
		// anime metadata response arrives.
		malID := r.URL.Query().Get("mal_id")
		if malID == "" {
			if resolved, err := h.listService.GetAnimeMALID(ctx, animeID); err == nil {
				malID = resolved
			}
		}
		if malID != "" {
			if entry, err := h.listService.GetUserAnimeEntry(ctx, userID, "mal_"+malID); err == nil && entry != nil {
				resp.WatchlistEntry = entry
			}
		}
	}

	if review, err := h.reviewService.GetUserReview(ctx, userID, animeID); err == nil && review != nil {
		projected := toReviewResponse(review)
		resp.MyReview = &projected
	}

	if pref, err := h.prefService.GetAnimePreference(ctx, userID, animeID); err == nil {
		resp.Combo = pref
	}

	httputil.OK(w, resp)
}
