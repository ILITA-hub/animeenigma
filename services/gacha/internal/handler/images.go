package handler

import (
	"context"
	"io"
	"net/http"
	"regexp"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	videoutils "github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/go-chi/chi/v5"
)

// imageReader is a thin adapter interface over *videoutils.Storage.Download so
// tests can inject a fake without importing the MinIO client.
type imageReader interface {
	Download(ctx context.Context, key string) (io.ReadCloser, *videoutils.VideoFile, error)
}

// validKeyPattern allows only safe object keys: cards/ or banners/ prefix,
// then URL-safe characters. Rejects path traversal (.. is absent from [A-Za-z0-9._-]).
var validKeyPattern = regexp.MustCompile(`^(cards|banners)/[A-Za-z0-9._-]+$`)

// ImagesHandler serves the public /images/* route for card/banner art.
// No auth is required: keys are unguessable UUIDs and the content is anime art.
type ImagesHandler struct {
	store imageReader
	log   *logger.Logger
}

func NewImagesHandler(store imageReader, log *logger.Logger) *ImagesHandler {
	return &ImagesHandler{store: store, log: log}
}

// Serve handles GET /images/*
func (h *ImagesHandler) Serve(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")

	// Guard against path traversal and invalid characters.
	if !validKeyPattern.MatchString(key) {
		http.NotFound(w, r)
		return
	}

	rc, meta, err := h.store.Download(r.Context(), key)
	if err != nil {
		// Any error is treated as a 404 (the object doesn't exist or isn't accessible).
		h.log.Infow("image not found", "key", key, "error", err)
		http.NotFound(w, r)
		return
	}
	defer rc.Close()

	ct := "application/octet-stream"
	if meta != nil && meta.ContentType != "" {
		ct = meta.ContentType
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if _, err := io.Copy(w, rc); err != nil {
		h.log.Warnw("image stream error", "key", key, "error", err)
	}
}

