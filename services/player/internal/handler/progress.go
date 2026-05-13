package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type ProgressHandler struct {
	progressService *service.ProgressService
	log             *logger.Logger
}

func NewProgressHandler(progressService *service.ProgressService, log *logger.Logger) *ProgressHandler {
	return &ProgressHandler{
		progressService: progressService,
		log:             log,
	}
}

// UpdateProgress updates watch progress for an episode
func (h *ProgressHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateProgressRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.Player != "" && !domain.ValidateCombo(req.Player, req.Language, req.WatchType) {
		httputil.Error(w, errors.InvalidInput("invalid combo fields: player, language, or watch_type"))
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	progress, err := h.progressService.UpdateProgress(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	metrics.WatchProgressSavesTotal.Inc()
	httputil.OK(w, progress)
}

// GetProgress returns watch progress for an anime
func (h *ProgressHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	progress, err := h.progressService.GetProgress(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, progress)
}

// MarkDropOff is the drop-off beacon endpoint. Receives a navigator.sendBeacon
// payload from the player when the user closes the page mid-episode. Records
// where the user stopped so Phase 6 Tier 2 inference and future analytics can
// distinguish "completed" from "abandoned at minute X". Phase 5 (G-01).
//
// Beacons typically arrive with Content-Type "text/plain;charset=UTF-8" or
// no header at all — httputil.Bind tolerates both because it only inspects
// the body when present.
func (h *ProgressHandler) MarkDropOff(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	var req domain.DropOffRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if req.EpisodeNumber <= 0 {
		httputil.Error(w, errors.InvalidInput("episode_number must be > 0"))
		return
	}
	if req.Progress < 0 {
		httputil.Error(w, errors.InvalidInput("progress must be >= 0"))
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	if err := h.progressService.MarkDropOff(r.Context(), claims.UserID, animeID, &req); err != nil {
		// Beacon endpoints can't surface errors to the user (the page is
		// already gone). Log and return success so the browser doesn't
		// retry — there's nothing the client can do about the failure.
		h.log.Errorw("failed to record dropoff",
			"user_id", claims.UserID,
			"anime_id", animeID,
			"episode", req.EpisodeNumber,
			"progress_secs", req.Progress,
			"error", err,
		)
	}
	metrics.WatchProgressSavesTotal.Inc()
	httputil.OK(w, map[string]bool{"recorded": true})
}

// ListContinueWatching returns the Continue-Watching row for the
// authenticated user — at most `limit` (default 10, max 50) anime, one
// row per anime, ordered by last_watched_at DESC. Phase 8 (UX-15 / UA-061).
//
// WR-01 (Phase 8): handler enforces an upper bound on `limit` and rejects
// values that exceed it with 400 BadRequest. The repo also clamps to
// [1, 20] as defense-in-depth — a future refactor that inlines the repo
// logic, or a new caller, won't accidentally pass unbounded pagination
// straight through to LIMIT ?. The handler is the canonical API boundary
// for input validation in this codebase (see domain/watch.go
// PaginationParams.Validate).
func (h *ProgressHandler) ListContinueWatching(w http.ResponseWriter, r *http.Request) {
	const (
		defaultLimit = 10
		maxLimit     = 50
	)

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			httputil.BadRequest(w, "limit must be a positive integer")
			return
		}
		if n > maxLimit {
			httputil.BadRequest(w, "limit must be <= 50")
			return
		}
		limit = n
	}

	items, err := h.progressService.ListContinueWatching(r.Context(), claims.UserID, limit)
	if err != nil {
		h.log.Errorw("failed to list continue-watching",
			"user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	// Always return a non-nil slice so the frontend can safely call .length.
	if items == nil {
		items = []*domain.ContinueWatchingItem{}
	}
	httputil.OK(w, items)
}

// GetBulkProgress returns the bulk per-anime progress map for the
// authenticated user, scoped to the comma-separated `ids` query param.
// Caps at 50 IDs per request — the AnimeCardNew composable batches per
// visible grid page. Phase 9 (UX-16).
func (h *ProgressHandler) GetBulkProgress(w http.ResponseWriter, r *http.Request) {
	const maxIDs = 50

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	raw := r.URL.Query().Get("ids")
	if raw == "" {
		httputil.OK(w, domain.BulkAnimeProgressMap{})
		return
	}
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ids = append(ids, p)
	}
	if len(ids) == 0 {
		httputil.OK(w, domain.BulkAnimeProgressMap{})
		return
	}
	if len(ids) > maxIDs {
		httputil.BadRequest(w, "ids must contain at most 50 entries")
		return
	}

	out, err := h.progressService.GetBulkProgress(r.Context(), claims.UserID, ids)
	if err != nil {
		h.log.Errorw("failed to bulk-load anime progress",
			"user_id", claims.UserID, "count", len(ids), "error", err)
		httputil.Error(w, err)
		return
	}
	if out == nil {
		out = domain.BulkAnimeProgressMap{}
	}
	httputil.OK(w, out)
}
