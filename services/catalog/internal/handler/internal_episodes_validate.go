package handler

// Watch-Together workstream / Phase 04 — WT-STATE-02.
//
// GET /internal/anime/{shikimoriId}/episodes/validate
//   ?player=kodik|animelib|ourenglish|hanime|raw
//   &episode_id=<provider-specific>
//   &translation_id=<provider-specific>       (optional — empty = player-change mode)
//   &watch_type=sub|dub                       (animelib only)
//
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as the sibling /internal/anime/
// {shikimoriId}/episodes (NOTIF-DET-01 / D-DET-02). nginx/gateway does
// NOT proxy /internal/*, so this endpoint is only reachable from inside
// the docker network.
//
// Response 200 always (for the soft-negative cases), e.g.:
//   {"valid":true,"reason":""}
//   {"valid":false,"reason":"EPISODE_UNAVAILABLE"}
//   {"valid":false,"reason":"PLAYER_UNAVAILABLE"}
//   {"valid":false,"reason":"TRANSLATION_UNAVAILABLE"}
//
// Response 400 — empty/invalid shikimoriId path param OR unknown
// player query param.
// Response 500 — repo/parser/cache infrastructure failure.
//
// Design references:
//   - .planning/workstreams/watch-together/phases/04-state-switching/04.1-PLAN.md
//   - .planning/workstreams/watch-together/phases/04-state-switching/04-CONTEXT.md
//   - docs/superpowers/specs/2026-05-25-watch-together-design.md
//
// Sibling endpoint /internal/anime/{shikimoriId}/episodes (notifications
// detector — NOTIF-DET-01) is NOT modified — its contract stays frozen.

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// EpisodesValidator is the narrow surface the handler needs from
// EpisodesValidateService. Tests inject a fake; the production
// *service.EpisodesValidateService satisfies it structurally.
type EpisodesValidator interface {
	ValidateEpisode(
		ctx context.Context,
		shikimoriID, player, episodeID, translationID, watchType string,
	) (service.ValidateResult, error)
}

// InternalEpisodesValidateHandler implements GET /internal/anime/
// {shikimoriId}/episodes/validate.
type InternalEpisodesValidateHandler struct {
	svc EpisodesValidator
	log *logger.Logger
}

// NewInternalEpisodesValidateHandler constructs the handler.
func NewInternalEpisodesValidateHandler(svc EpisodesValidator, log *logger.Logger) *InternalEpisodesValidateHandler {
	return &InternalEpisodesValidateHandler{svc: svc, log: log}
}

// Validate handles GET /internal/anime/{shikimoriId}/episodes/validate.
func (h *InternalEpisodesValidateHandler) Validate(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimoriId")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimoriId is required")
		return
	}
	// Reuse the strict regex declared in internal_cache.go — rejects
	// path-traversal and injection-shaped input before it can reach
	// any downstream layer.
	if !shikimoriIDPattern.MatchString(shikimoriID) {
		httputil.BadRequest(w, "shikimoriId has invalid characters")
		return
	}

	q := r.URL.Query()
	player := q.Get("player")
	episodeID := q.Get("episode_id")
	translationID := q.Get("translation_id")
	watchType := q.Get("watch_type")

	if player == "" {
		httputil.BadRequest(w, "player is required")
		return
	}
	// Pre-validate at the HTTP edge so callers get a fast 400 for the
	// unknown-player case (service would also return InvalidInput, but
	// this saves a service hop and keeps the handler symmetric with
	// the sibling internal_episodes.go).
	if !service.IsValidPlayer(player) {
		httputil.BadRequest(w, "player not supported")
		return
	}

	result, err := h.svc.ValidateEpisode(r.Context(), shikimoriID, player, episodeID, translationID, watchType)
	if err != nil {
		if h.log != nil {
			h.log.Debugw("internal episodes validate error",
				"shikimori_id", shikimoriID,
				"player", player,
				"episode_id", episodeID,
				"translation_id", translationID,
				"watch_type", watchType,
				"error", err,
			)
		}
		// Service returns AppError with the right code; httputil.Error
		// maps to the right HTTP status (400/404/500).
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, result)
}
