package handler

import (
	"net/http"
	"sync/atomic"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"golang.org/x/sync/semaphore"
)

// HLS proxy configuration
const (
	maxHLSProxyConnections = 50 // Maximum concurrent HLS proxy streams
)

// Global state for HLS proxy connection limiting
var (
	hlsProxySemaphore    = semaphore.NewWeighted(maxHLSProxyConnections)
	hlsActiveConnections atomic.Int32
)

type StreamHandler struct {
	streamingService *service.StreamingService
	videoProxy       *videoutils.VideoProxy
	log              *logger.Logger
}

func NewStreamHandler(streamingService *service.StreamingService, log *logger.Logger) *StreamHandler {
	// Create video proxy with default config for HLS proxying
	proxyCfg := videoutils.DefaultProxyConfig()
	proxyCfg.AllowedDomains = videoutils.HLSProxyAllowedDomains

	return &StreamHandler{
		streamingService: streamingService,
		videoProxy:       videoutils.NewVideoProxy(proxyCfg),
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

// HLSProxy proxies HLS streams with proper Referer headers
// This endpoint allows the frontend to play HLS streams that require Referer authentication
func (h *StreamHandler) HLSProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get query parameters
	sourceURL := r.URL.Query().Get("url")
	referer := r.URL.Query().Get("referer")

	if sourceURL == "" {
		httputil.BadRequest(w, "url parameter is required")
		return
	}

	// Try to acquire semaphore (limit concurrent connections)
	if !hlsProxySemaphore.TryAcquire(1) {
		h.log.Warnw("HLS proxy at capacity", "active_connections", hlsActiveConnections.Load())
		w.Header().Set("Retry-After", "30")
		httputil.Error(w, errors.ServiceUnavailable("server busy, try again later"))
		return
	}

	// Track active connections
	hlsActiveConnections.Add(1)
	defer func() {
		hlsProxySemaphore.Release(1)
		hlsActiveConnections.Add(-1)
	}()

	h.log.Debugw("HLS proxy request",
		"url", sourceURL,
		"referer", referer,
		"active_connections", hlsActiveConnections.Load(),
	)

	// Proxy the request with the provided referer
	if err := h.videoProxy.ProxyWithReferer(r.Context(), sourceURL, referer, w, r); err != nil {
		h.log.Errorw("failed to proxy HLS stream",
			"error", err,
			"url", sourceURL,
			"referer", referer,
		)
		// Don't send error response if headers already sent
	}
}

// GetProxyStatus returns the current HLS proxy load status
func (h *StreamHandler) GetProxyStatus(w http.ResponseWriter, r *http.Request) {
	active := hlsActiveConnections.Load()
	loadPercent := int(float64(active) / float64(maxHLSProxyConnections) * 100)

	httputil.OK(w, map[string]interface{}{
		"active_connections": active,
		"max_connections":    maxHLSProxyConnections,
		"load_percent":       loadPercent,
		"available":          active < maxHLSProxyConnections,
	})
}
