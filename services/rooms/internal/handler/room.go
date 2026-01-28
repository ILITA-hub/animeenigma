package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
	"github.com/go-chi/chi/v5"
)

type RoomHandler struct {
	roomService *service.RoomService
	log         *logger.Logger
}

func NewRoomHandler(roomService *service.RoomService, log *logger.Logger) *RoomHandler {
	return &RoomHandler{
		roomService: roomService,
		log:         log,
	}
}

// CreateRoom creates a new game room
func (h *RoomHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRoomRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims := authz.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.Unauthorized(w)
		return
	}

	room, err := h.roomService.CreateRoom(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.Created(w, room)
}

// ListRooms returns all available rooms
func (h *RoomHandler) ListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.roomService.ListRooms(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, rooms)
}

// GetRoom returns a specific room
func (h *RoomHandler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		httputil.Error(w, errors.InvalidInput("room_id is required"))
		return
	}

	room, err := h.roomService.GetRoom(r.Context(), roomID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, room)
}

// JoinRoom allows a user to join a room
func (h *RoomHandler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		httputil.Error(w, errors.InvalidInput("room_id is required"))
		return
	}

	claims := authz.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.Unauthorized(w)
		return
	}

	err := h.roomService.JoinRoom(r.Context(), roomID, claims.UserID, claims.Username)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"status": "joined"})
}

// LeaveRoom allows a user to leave a room
func (h *RoomHandler) LeaveRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		httputil.Error(w, errors.InvalidInput("room_id is required"))
		return
	}

	claims := authz.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.Unauthorized(w)
		return
	}

	err := h.roomService.LeaveRoom(r.Context(), roomID, claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}
