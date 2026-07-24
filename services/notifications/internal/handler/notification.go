package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/service"
	"github.com/go-chi/chi/v5"
)

// NotificationHandler serves the 7 public CRUD endpoints under
// /api/notifications. All handlers extract user_id from JWT claims via
// authz.UserIDFromContext — NOT from any X-User-ID header (D-03).
type NotificationHandler struct {
	svc *service.NotificationService
	log *logger.Logger
}

// NewNotificationHandler constructs the handler.
func NewNotificationHandler(svc *service.NotificationService, log *logger.Logger) *NotificationHandler {
	return &NotificationHandler{svc: svc, log: log}
}

// ListResponse is the JSON shape returned by GET /api/notifications.
// One round-trip surfaces the rows plus both counts the bell badge needs.
type ListResponse struct {
	Notifications []domain.UserNotification `json:"notifications"`
	UnreadCount   int64                     `json:"unread_count"`
	Total         int64                     `json:"total"`
}

// UnreadCountResponse is the shape returned by GET /api/notifications/unread-count.
type UnreadCountResponse struct {
	UnreadCount int64 `json:"unread_count"`
}

// MarkAllReadResponse is the shape returned by POST /api/notifications/mark-all-read.
type MarkAllReadResponse struct {
	Updated int64 `json:"updated"`
}

// List handles GET /api/notifications?status=unread|all|history&limit=20&offset=0
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}

	status := repo.ParseListStatus(r.URL.Query().Get("status"))

	limit := parseIntQuery(r, "limit", 20)
	offset := parseIntQuery(r, "offset", 0)

	rows, unread, total, err := h.svc.List(r.Context(), userID, status, limit, offset)
	if err != nil {
		h.log.Errorw("list notifications failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}

	// Replace nil with an empty slice so JSON callers never see `null`.
	if rows == nil {
		rows = []domain.UserNotification{}
	}

	httputil.OK(w, ListResponse{
		Notifications: rows,
		UnreadCount:   unread,
		Total:         total,
	})
}

// UnreadCount handles GET /api/notifications/unread-count.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}

	n, err := h.svc.UnreadCount(r.Context(), userID)
	if err != nil {
		h.log.Errorw("unread count failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, UnreadCountResponse{UnreadCount: n})
}

// MarkRead handles POST /api/notifications/{id}/read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "missing notification id")
		return
	}

	if err := h.svc.MarkRead(r.Context(), userID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "ok"})
}

// MarkAllRead handles POST /api/notifications/mark-all-read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}

	n, err := h.svc.MarkAllRead(r.Context(), userID)
	if err != nil {
		h.log.Errorw("mark all read failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, MarkAllReadResponse{Updated: n})
}

// Dismiss handles POST /api/notifications/{id}/dismiss.
func (h *NotificationHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "missing notification id")
		return
	}

	if err := h.svc.Dismiss(r.Context(), userID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "ok"})
}

// Delete handles POST /api/notifications/{id}/delete — the "bin" action in
// the All-notifications history modal. Unlike Dismiss, a deleted notification
// disappears from every surface (unread, all, history).
func (h *NotificationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "missing notification id")
		return
	}

	if err := h.svc.Delete(r.Context(), userID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "ok"})
}

// Click handles POST /api/notifications/{id}/click. Body is intentionally
// empty — the click event is fire-and-forget telemetry, the frontend
// navigates on its own and doesn't wait for a meaningful response.
func (h *NotificationHandler) Click(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "missing notification id")
		return
	}

	if err := h.svc.Click(r.Context(), userID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "ok"})
}

func parseIntQuery(r *http.Request, key string, defaultVal int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return defaultVal
}
