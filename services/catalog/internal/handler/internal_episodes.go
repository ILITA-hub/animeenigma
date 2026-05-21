package handler

// Phase 2 v1.0 Notifications Engine — NOTIF-DET-01.
//
// GET /internal/anime/{shikimoriId}/episodes
//   ?player=kodik|animelib
//   &translation_id=<provider-specific>
//   &watch_type=sub|dub
//   &language=<bcp47 short>
//
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as POST /internal/cache/invalidate/raw/
// (see internal_cache.go for the precedent + design-doc D-DET-02).
//
// Response 200:
//   {
//     "latest_available_episode": 12,
//     "checked_at": "2026-05-21T03:00:00Z"
//   }
//
// Response 400 — player not in the v1.0 allowlist {kodik, animelib} or a
// required query param missing.
// Response 404 — combo has no matching upstream episode (parser-level
// not-found).
// Response 500 — parser/HTTP/cache infrastructure failure.
//
// Idempotent + cacheable: a second identical call within 5 minutes is
// served from Redis with the cache key
// `notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}`.

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// InternalEpisodesHandler implements GET
// /internal/anime/{shikimoriId}/episodes. The handler is a thin shell over
// EpisodesLookupService — input validation + JSON shape, nothing else. All
// caching, parser dispatch, and error classification live in the service.
type InternalEpisodesHandler struct {
	svc *service.EpisodesLookupService
	log *logger.Logger
}

// NewInternalEpisodesHandler constructs the handler.
func NewInternalEpisodesHandler(svc *service.EpisodesLookupService, log *logger.Logger) *InternalEpisodesHandler {
	return &InternalEpisodesHandler{svc: svc, log: log}
}

// GetLatestEpisode handles GET /internal/anime/{shikimoriId}/episodes.
func (h *InternalEpisodesHandler) GetLatestEpisode(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimoriId")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimoriId is required")
		return
	}
	// Reuse the same strict regex as InvalidateRaw — rejects path-traversal
	// or injection-shaped input long before the value reaches Redis or the
	// parser.
	if !shikimoriIDPattern.MatchString(shikimoriID) {
		httputil.BadRequest(w, "shikimoriId has invalid characters")
		return
	}

	q := r.URL.Query()
	player := q.Get("player")
	translationID := q.Get("translation_id")
	watchType := q.Get("watch_type")
	language := q.Get("language")

	if player != "kodik" && player != "animelib" {
		// D-DET-03: any other player value returns 400 in v1.0.
		httputil.BadRequest(w, "player not supported by detector in v1.0")
		return
	}
	if translationID == "" {
		httputil.BadRequest(w, "translation_id is required")
		return
	}

	result, err := h.svc.LatestAvailable(r.Context(), shikimoriID, player, translationID, watchType, language)
	if err != nil {
		// Service returns AppError with the right code; httputil.Error
		// maps to the right HTTP status (400/404/500).
		if h.log != nil {
			h.log.Debugw("internal episodes lookup error",
				"shikimori_id", shikimoriID,
				"player", player,
				"translation_id", translationID,
				"watch_type", watchType,
				"error", err,
			)
		}
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, result)
}
