package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
)

type StreamHandler struct {
	streamingService *service.StreamingService
	log              *logger.Logger
}

func NewStreamHandler(streamingService *service.StreamingService, log *logger.Logger) *StreamHandler {
	return &StreamHandler{
		streamingService: streamingService,
		log:              log,
	}
}

// ProxyStream handles proxying external video streams
func (h *StreamHandler) ProxyStream(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		httputil.BadRequest(w, "token is required")
		return
	}

	token, err := h.streamingService.ValidateStreamToken(tokenStr)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	switch token.SourceType {
	case videoutils.SourceExternal:
		if err := h.streamingService.ProxyExternalStream(r.Context(), token, w, r); err != nil {
			h.log.Errorw("failed to proxy stream", "error", err, "video_id", token.VideoID)
			// Don't send error response if we've already started writing
		}

	case videoutils.SourceMinio:
		if err := h.streamingService.StreamFromStorage(r.Context(), token, w, r); err != nil {
			h.log.Errorw("failed to stream from storage", "error", err, "video_id", token.VideoID)
		}

	default:
		httputil.Error(w, errors.InvalidInput("unsupported source type"))
	}
}

// DirectStream handles direct streaming from MinIO storage
func (h *StreamHandler) DirectStream(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		httputil.BadRequest(w, "token is required")
		return
	}

	token, err := h.streamingService.ValidateStreamToken(tokenStr)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if token.SourceType != videoutils.SourceMinio {
		httputil.Error(w, errors.InvalidInput("token is not for direct streaming"))
		return
	}

	if err := h.streamingService.StreamFromStorage(r.Context(), token, w, r); err != nil {
		h.log.Errorw("failed to stream from storage", "error", err, "video_id", token.VideoID)
	}
}

// GenerateToken generates a stream token (internal API)
func (h *StreamHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VideoID    string `json:"video_id"`
		SourceType string `json:"source_type"`
		SourceURL  string `json:"source_url,omitempty"`
		StorageKey string `json:"storage_key,omitempty"`
		UserID     string `json:"user_id,omitempty"`
	}

	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	var sourceType videoutils.VideoSource
	switch req.SourceType {
	case "minio":
		sourceType = videoutils.SourceMinio
	case "external":
		sourceType = videoutils.SourceExternal
	default:
		httputil.BadRequest(w, "invalid source_type")
		return
	}

	token, expiresAt, err := h.streamingService.GenerateStreamToken(
		req.VideoID, sourceType, req.SourceURL, req.StorageKey, req.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt,
	})
}
