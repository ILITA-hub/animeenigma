package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/service"
)

// InternalHandler serves the internal producer endpoints under /internal/*.
//
// These routes are NOT protected by middleware in this service. Security
// model (D-05): the gateway never proxies /internal/* — so the only
// reachable callers are processes already inside the Docker network
// (Phase 2's detector running inside the same service, or operators using
// `docker compose exec`). This matches the established project pattern —
// see services/auth/internal/transport/router.go which mounts
// /internal/resolve-api-key the same way.
type InternalHandler struct {
	svc *service.NotificationService
	log *logger.Logger
}

// NewInternalHandler constructs the handler.
func NewInternalHandler(svc *service.NotificationService, log *logger.Logger) *InternalHandler {
	return &InternalHandler{svc: svc, log: log}
}

// CreateNotification handles POST /internal/notifications.
//
// Body: { "user_id", "type", "dedupe_key", "payload" } per
// service.UpsertRequest. Payload is raw JSON — the service validates it
// is well-formed before persisting. Returns the resulting UserNotification
// row (post-UPSERT canonical state).
func (h *InternalHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var req service.UpsertRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	row, err := h.svc.Upsert(r.Context(), req)
	if err != nil {
		h.log.Errorw("internal upsert notification failed",
			"user_id", req.UserID,
			"type", req.Type,
			"dedupe_key", req.DedupeKey,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, row)
}

// Health handles GET /internal/health. Equivalent to the public /health
// route but never proxied — used by internal callers (the cron detector in
// Phase 2 will probe this before submitting a batch).
func (h *InternalHandler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
