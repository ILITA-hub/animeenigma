// Package handler holds the chi-compatible HTTP handlers for the
// watch-together service. v1.0 Plan 01.4 ships /rooms CRUD; the WebSocket
// upgrader lands in 01.5, inbound message router in 01.6.
//
// All handlers extract user_id from JWT claims via authz.UserIDFromContext —
// NEVER from request headers or query params (mirrors services/notifications
// D-03). The AuthMiddleware applied at the route group in internal/transport
// is responsible for short-circuiting unauthenticated callers; handlers
// double-check defensively so a future middleware re-order can't silently
// expose endpoints.
package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
)

// RoomDeleteFanout is the narrow surface of *hub.Hub the Delete handler
// uses to fan out a room:closed envelope before deleting Redis keys.
// Mirrors service.HubFanout's Broadcast signature; defined here so
// rooms_test.go can pass a fake. The real *hub.Hub satisfies this by
// signature (verified at compile time in main.go).
type RoomDeleteFanout interface {
	Broadcast(ctx context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error)
}

// RoomHandler serves POST/GET/DELETE /api/watch-together/rooms[/{id}].
// State lives in the service layer — this struct holds no Redis client and
// performs no validation beyond JSON decoding (validate lives in
// service.CreateRoomInput.validate). Config is read for PublicBaseURL only.
//
// hub + graceMgr (Plan 05.1) are consulted by Delete to fan out a
// room:closed envelope to connected members and cancel any pending
// grace timer BEFORE the explicit DeleteRoom. Closes the 01-04-SUMMARY.md
// deviation #5 (host DELETE used to silently delete without notifying
// connected members).
type RoomHandler struct {
	svc      *service.RoomService
	hub      RoomDeleteFanout
	graceMgr GraceCanceller
	cfg      *config.Config
	log      *logger.Logger
}

// NewRoomHandler wires the service + config + logger. Pass nil for log to
// fall back to logger.Default() inside the handler methods.
//
// hub + graceMgr (Plan 05.1) may both be nil for legacy / test wiring;
// when present, Delete broadcasts room:closed and cancels any pending
// grace timer before tearing the room down.
func NewRoomHandler(svc *service.RoomService, hub RoomDeleteFanout, graceMgr GraceCanceller, cfg *config.Config, log *logger.Logger) *RoomHandler {
	if log == nil {
		log = logger.Default()
	}
	return &RoomHandler{svc: svc, hub: hub, graceMgr: graceMgr, cfg: cfg, log: log}
}

// CreateRoomBody is the JSON request shape for POST /rooms. All 4 fields
// are required; missing/blank field → 400 BadRequest with explicit reason.
// Player must be one of the 5 domain.Player* constants (service layer
// rejects unknown values).
type CreateRoomBody struct {
	AnimeID       string `json:"anime_id"`
	EpisodeID     string `json:"episode_id"`
	Player        string `json:"player"`
	TranslationID string `json:"translation_id"`
}

// CreateRoomResponse is the JSON shape returned by POST /rooms. Kept narrow
// per the design doc §API: only fields the frontend needs to (a) navigate
// to the room view and (b) open the WebSocket. RoomSnapshot lives behind
// GET /rooms/{id} so a client interested in the full state pulls it after
// landing on the room page.
type CreateRoomResponse struct {
	RoomID    string `json:"room_id"`
	InviteURL string `json:"invite_url"`
	WSUrl     string `json:"ws_url"`
}

// Create handles POST /api/watch-together/rooms. Auth-middleware-protected;
// extracts userID + username from JWT claims and delegates to RoomService.
func (h *RoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		// Defensive — AuthMiddleware should have short-circuited.
		httputil.Unauthorized(w)
		return
	}
	// Guest containment: a Watch Together guest (ephemeral invite-link
	// identity, role=guest) may JOIN a room — GET /rooms/{id} + the WS —
	// but MUST NOT create one. The gateway intentionally lets guest tokens
	// reach the WT routes (so join works), so room-creation is gated here
	// at the service. Mirrors the gateway's BlockGuestRoleMiddleware which
	// rejects guests on every OTHER protected route.
	if authz.RoleFromContext(r.Context()) == authz.RoleGuest {
		httputil.Forbidden(w)
		return
	}
	username := ""
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok {
		username = claims.Username
	}

	var body CreateRoomBody
	if err := httputil.Bind(r, &body); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}

	room, err := h.svc.Create(r.Context(), userID, username, service.CreateRoomInput{
		AnimeID:       body.AnimeID,
		EpisodeID:     body.EpisodeID,
		Player:        body.Player,
		TranslationID: body.TranslationID,
	})
	if err != nil {
		if stderrors.Is(err, service.ErrInvalidInput) {
			httputil.BadRequest(w, err.Error())
			return
		}
		h.log.Errorw("watch_together create_room failed",
			"user_id", userID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	resp := CreateRoomResponse{
		RoomID:    room.ID,
		InviteURL: inviteURL(h.cfg.PublicBaseURL, room.ID),
		WSUrl:     wsURLFromBase(h.cfg.PublicBaseURL, room.ID),
	}

	h.log.Infow("watch_together create_room ok",
		"action", "create_room",
		"room_id", room.ID,
		"user_id", userID,
	)
	httputil.OK(w, resp)
}

// Get handles GET /api/watch-together/rooms/{id}. Returns the RoomSnapshot
// (room state + members + last 50 messages + protocol_version) for a live
// room, or 410 Gone if the room's TTL has expired / it was deleted by host.
func (h *RoomHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}

	roomID := chi.URLParam(r, "id")
	if roomID == "" {
		httputil.BadRequest(w, "missing room id")
		return
	}

	snap, err := h.svc.Get(r.Context(), roomID)
	if err != nil {
		if stderrors.Is(err, service.ErrNotFound) {
			writeGone(w, "room expired or does not exist")
			h.log.Infow("watch_together get_room gone",
				"action", "get_room",
				"room_id", roomID,
				"user_id", userID,
			)
			return
		}
		if stderrors.Is(err, service.ErrInvalidInput) {
			httputil.BadRequest(w, err.Error())
			return
		}
		h.log.Errorw("watch_together get_room failed",
			"action", "get_room",
			"room_id", roomID,
			"user_id", userID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	h.log.Infow("watch_together get_room ok",
		"action", "get_room",
		"room_id", roomID,
		"user_id", userID,
		"member_count", len(snap.Members),
		"message_count", len(snap.Messages),
	)
	httputil.OK(w, snap)
}

// Delete handles DELETE /api/watch-together/rooms/{id}. Only the host
// (room.host_user_id == requester user_id) can force-close (WT-FOUND-03).
// 204 on success, 403 ErrNotHost, 410 ErrNotFound.
//
// Plan 05.1 — closes 01-04-SUMMARY.md deviation #5: BEFORE deleting Redis
// keys, we cancel any pending grace timer (so its fire callback can't
// double-broadcast) and fan out a room:closed envelope to every
// connected member so they see an explicit close event (not a sudden
// 410 on the next REST call). The broadcast happens AFTER the authz
// preflight (svc.Get) + host check, so non-host attempts cannot spam the
// channel and gone rooms cannot trigger broadcasts.
func (h *RoomHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}

	roomID := chi.URLParam(r, "id")
	if roomID == "" {
		httputil.BadRequest(w, "missing room id")
		return
	}

	// Plan 05.1 preflight — peek the room so we can broadcast room:closed
	// AND cancel the grace timer BEFORE the explicit DeleteRoom. The
	// host-check below is the authz gate; svc.Delete still re-validates
	// host-ownership (defense in depth — service is the single mutation
	// surface, handler-side checks could go stale under future refactors).
	snap, err := h.svc.Get(r.Context(), roomID)
	if err != nil {
		if stderrors.Is(err, service.ErrNotFound) {
			writeGone(w, "room expired or does not exist")
			h.log.Infow("watch_together delete_room gone",
				"action", "delete_room",
				"room_id", roomID,
				"user_id", userID,
			)
			return
		}
		if stderrors.Is(err, service.ErrInvalidInput) {
			httputil.BadRequest(w, err.Error())
			return
		}
		h.log.Errorw("watch_together delete_room peek failed",
			"action", "delete_room",
			"room_id", roomID,
			"user_id", userID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}
	if snap.Room.HostUserID != userID {
		httputil.Forbidden(w)
		h.log.Infow("watch_together delete_room not_host",
			"action", "delete_room",
			"room_id", roomID,
			"user_id", userID,
		)
		return
	}

	// Authz cleared — cancel any pending grace timer so its fire callback
	// can't race us into broadcasting a duplicate room:closed, then fan
	// out the close event so connected members see it explicitly.
	if h.graceMgr != nil {
		h.graceMgr.Cancel(roomID)
	}
	if h.hub != nil {
		closedEnv := domain.Envelope{
			Type: domain.MsgRoomClosed,
			Data: []byte(`{}`),
		}
		if _, berr := h.hub.Broadcast(r.Context(), roomID, closedEnv, ""); berr != nil {
			h.log.Warnw("watch_together delete_room broadcast room_closed",
				"room_id", roomID,
				"user_id", userID,
				"err", berr,
			)
		}
	}

	if err := h.svc.Delete(r.Context(), userID, roomID); err != nil {
		switch {
		case stderrors.Is(err, service.ErrNotFound):
			// Race: room expired between the peek above and Delete. Treat
			// as 410 — the broadcast already went out, so connected
			// members got a close event regardless.
			writeGone(w, "room expired or does not exist")
		case stderrors.Is(err, service.ErrNotHost):
			// Race: host changed between peek and Delete. Effectively
			// never happens in v1.0 (host_user_id is set once at Create
			// and never updated), but defensive.
			httputil.Forbidden(w)
		case stderrors.Is(err, service.ErrInvalidInput):
			httputil.BadRequest(w, err.Error())
		default:
			h.log.Errorw("watch_together delete_room failed",
				"action", "delete_room",
				"room_id", roomID,
				"user_id", userID,
				"error", err,
			)
			httputil.Error(w, err)
		}
		return
	}

	h.log.Infow("watch_together delete_room ok",
		"action", "delete_room",
		"room_id", roomID,
		"user_id", userID,
	)
	httputil.NoContent(w)
}

// inviteURL constructs the browser-navigable URL for a room. Kept as a
// package-private helper so the same logic isn't duplicated between the
// production handler and tests that need to assert the response shape.
//
// PublicBaseURL is guaranteed trailing-slash-free by config.Load() so
// simple concatenation is correct.
func inviteURL(base, roomID string) string {
	return base + "/watch/room/" + roomID
}

// wsURLFromBase swaps the URL scheme of `base` from http(s) → ws(s) and
// appends the gateway-proxied WS path. Output is the URL the frontend
// passes to `new WebSocket(...)`.
//
// Rules (mirror common reverse-proxy conventions):
//
//	https://animeenigma.ru     → wss://animeenigma.ru/api/watch-together/ws?room=<id>
//	http://localhost:8000      → ws://localhost:8000/api/watch-together/ws?room=<id>
//	(no scheme)                → ws://<base>/...  (defensive fallback)
//
// We do NOT use net/url here — the input space is small and a one-line
// HasPrefix check is more robust against malformed inputs (a parse error
// on this path would be a config bug, not user input).
func wsURLFromBase(base, roomID string) string {
	var wsBase string
	switch {
	case strings.HasPrefix(base, "https://"):
		wsBase = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		wsBase = "ws://" + strings.TrimPrefix(base, "http://")
	default:
		// Defensive fallback — pretend it's plain http. Production config
		// always includes a scheme so this branch is unreachable in real
		// deployments, but keeps tests resilient to misconfiguration.
		wsBase = "ws://" + base
	}
	return wsBase + "/api/watch-together/ws?room=" + roomID
}

// writeGone writes a 410 Gone with the standard libs/httputil error
// envelope shape. libs/httputil has no Gone helper as of 2026-05, so this
// is a thin inline equivalent. Code "GONE" is a stable string the frontend
// can match against to decide between "room not found" UX and "your
// session expired" UX.
//
// Note: keep the body shape aligned with httputil.Error so frontend doesn't
// need a special case for 410 vs 404/403.
func writeGone(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	// Reusing the existing httputil.Response struct via the error envelope
	// would require constructing an AppError with StatusCode=410, which
	// libs/errors doesn't expose as a built-in. Inline the encoding to
	// keep the dependency surface flat.
	_, _ = w.Write([]byte(`{"success":false,"error":{"code":"GONE","message":"` + escapeJSONString(message) + `"}}`))
}

// escapeJSONString escapes the small set of characters that would break the
// inline-JSON encoding in writeGone. Inputs are author-controlled string
// constants today, but defensive escaping costs nothing and protects
// against future callers passing dynamic text.
func escapeJSONString(s string) string {
	// Minimal escaping — quotes and backslashes only. The message strings
	// we pass are ASCII so the spec-required \uXXXX cases don't apply.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
