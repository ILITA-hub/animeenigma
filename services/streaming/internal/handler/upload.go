package handler

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
)

type UploadHandler struct {
	streamingService *service.StreamingService
	log              *logger.Logger
}

func NewUploadHandler(streamingService *service.StreamingService, log *logger.Logger) *UploadHandler {
	return &UploadHandler{
		streamingService: streamingService,
		log:              log,
	}
}

// UploadVideo handles video file uploads
func (h *UploadHandler) UploadVideo(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 2GB max
	if err := r.ParseMultipartForm(2 << 30); err != nil {
		httputil.BadRequest(w, "failed to parse form")
		return
	}

	// Get form values
	animeID := r.FormValue("anime_id")
	episodeNumStr := r.FormValue("episode_number")
	quality := r.FormValue("quality")
	if quality == "" {
		quality = "720p"
	}

	if animeID == "" || episodeNumStr == "" {
		httputil.BadRequest(w, "anime_id and episode_number are required")
		return
	}

	episodeNum, err := strconv.Atoi(episodeNumStr)
	if err != nil {
		httputil.BadRequest(w, "invalid episode_number")
		return
	}

	// Get file
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.BadRequest(w, "file is required")
		return
	}
	defer file.Close()

	// Validate content type
	contentType := header.Header.Get("Content-Type")
	if contentType != "video/mp4" && contentType != "video/webm" && contentType != "video/x-matroska" {
		httputil.BadRequest(w, "unsupported video format")
		return
	}

	// Generate storage key
	storageKey := videoutils.GenerateVideoKey(animeID, strconv.Itoa(episodeNum), quality)

	// Upload to storage
	videoFile, err := h.streamingService.Upload(r.Context(), storageKey, file, header.Size, contentType)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("video uploaded",
		"anime_id", animeID,
		"episode", episodeNum,
		"quality", quality,
		"size", videoFile.Size,
		"key", storageKey)

	httputil.Created(w, map[string]interface{}{
		"video_id":       videoFile.Key,
		"anime_id":       animeID,
		"episode_number": episodeNum,
		"quality":        quality,
		"size":           videoFile.Size,
		"storage_key":    storageKey,
	})
}

// GetUploadURL generates a presigned URL for direct upload
func (h *UploadHandler) GetUploadURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AnimeID       string `json:"anime_id"`
		EpisodeNumber int    `json:"episode_number"`
		Filename      string `json:"filename"`
		ContentType   string `json:"content_type"`
		Size          int64  `json:"size"`
		Quality       string `json:"quality"`
	}

	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.AnimeID == "" || req.EpisodeNumber == 0 || req.Filename == "" {
		httputil.BadRequest(w, "anime_id, episode_number, and filename are required")
		return
	}

	if req.Quality == "" {
		req.Quality = "720p"
	}

	// Generate storage key
	ext := path.Ext(req.Filename)
	storageKey := fmt.Sprintf("videos/%s/ep%d_%s%s", req.AnimeID, req.EpisodeNumber, req.Quality, ext)

	// Presigned PUT URL — the client uploads the bytes straight to MinIO/S3 with
	// an HTTP PUT, bypassing this service. Valid for a short upload window.
	const uploadWindow = 15 * time.Minute
	uploadURL, err := h.streamingService.GetUploadURL(r.Context(), storageKey, uploadWindow)
	if err != nil {
		h.log.Errorw("failed to generate presigned upload url", "storage_key", storageKey, "error", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"upload_url":   uploadURL,
		"method":       http.MethodPut,
		"storage_key":  storageKey,
		"content_type": req.ContentType,
		"expires_at":   time.Now().Add(uploadWindow).UTC().Format(time.RFC3339),
	})
}

// DeleteVideo handles video deletion
func (h *UploadHandler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageKey string `json:"storage_key"`
	}

	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.StorageKey == "" {
		httputil.BadRequest(w, "storage_key is required")
		return
	}

	if err := h.streamingService.Delete(r.Context(), req.StorageKey); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("video deleted", "key", req.StorageKey)
	httputil.NoContent(w)
}

