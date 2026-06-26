package handler

// models.go — capability-signed worker model serving endpoint.
//
// GET /worker/models/{name} streams the model artifact from MinIO to the
// requesting worker. Auth is a name-bound HMAC capability handle (T25 contract)
// — the same pattern as the segment data-plane (segments.go). No JWT, no
// X-Gateway-Internal header; gating is the ?exp=&sig= handle bound to the model
// name and operation "model" with idx=0 (models are addressed by name, not index).
//
// Security controls (mirrors segments.go):
//  1. Traversal guard — reject name containing '/', '..', '\', or empty.
//  2. Capability verify — VerifyJobHandle(name, "model", 0, exp, sig, now).
//  3. 404 if no model found for name (generic body).
//  4. Clear write deadline before streaming (models can be hundreds of MiB).
//  5. Generic error bodies on all failures (no internal detail leaked).

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/go-chi/chi/v5"
)

// modelGetter is the minimal ModelRepository surface ModelServeHandler needs.
// Get(name, version) is already on ModelRepository; GetLatest is added by this
// task so the serve path can resolve the most-recent version by name alone.
type modelGetter interface {
	GetLatest(ctx context.Context, name string) (*domain.UpscaleModel, error)
}

// modelObjectGetter is the minimal MinIO interface needed for streaming model
// artifacts. Declared here as a test seam (mirrors the modelUploader pattern in
// model_admin.go). The production wiring passes upWriter.RawUploader() which
// satisfies the full minio.Uploader interface including GetObject.
type modelObjectGetter interface {
	GetObject(ctx context.Context, bucket, object string) (io.ReadCloser, error)
}

// ModelServeHandler handles GET /worker/models/{name}.
//
// The caller must hold a valid capability handle minted over the model name
// (T25: MintJobHandle(modelName, "model", 0, ttl)) — the same HMAC scheme as
// segments, bound to name+op+idx=0. A handle for model "realesrgan" cannot be
// used to fetch "waifu2x", and vice versa (name-binding).
type ModelServeHandler struct {
	repo     modelGetter
	uploader modelObjectGetter
	bucket   string
	log      *logger.Logger
}

// NewModelServeHandler constructs a ModelServeHandler.
// uploader is typically the *minio.Writer's underlying RawUploader().
// bucket is the MinIO bucket (e.g. cfg.Upscaler.MinIO.Bucket).
func NewModelServeHandler(repo modelGetter, uploader modelObjectGetter, bucket string, log *logger.Logger) *ModelServeHandler {
	if log == nil {
		log = logger.Default()
	}
	if bucket == "" {
		bucket = "raw-library"
	}
	return &ModelServeHandler{repo: repo, uploader: uploader, bucket: bucket, log: log}
}

// writeNotFound writes a generic 404 with no internal detail.
func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	//nolint:errcheck
	w.Write([]byte(`{"error":"not found"}`))
}

// ServeModel handles GET /worker/models/{name}.
func (h *ModelServeHandler) ServeModel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// ── Security ctrl #1: traversal guard ──────────────────────────────────
	// Reject names that could escape the models/{name}/ key prefix.
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		writeBadRequest(w)
		return
	}

	// ── Security ctrl #2: capability verify (name-bound) ───────────────────
	// Handle minted by T25 as MintJobHandle(modelName, "model", 0, ttl).
	// VerifyJobHandle is fail-closed: returns false when secret is unconfigured.
	exp := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if !capability.VerifyJobHandle(name, "model", 0, exp, sig, time.Now()) {
		writeUnauthorized(w)
		return
	}

	// ── Security ctrl #3: model lookup ─────────────────────────────────────
	// Resolve the latest version registered for this name.
	model, err := h.repo.GetLatest(r.Context(), name)
	if err != nil {
		if isNotFound(err) {
			writeNotFound(w)
		} else {
			h.log.Errorw("models: GetLatest failed", "name", name, "error", err)
			writeInternalError(w)
		}
		return
	}
	if model == nil {
		// GetLatest returned (nil, nil) — no rows for this name.
		writeNotFound(w)
		return
	}

	// ── Security ctrl #4: stream from MinIO ────────────────────────────────
	rc, err := h.uploader.GetObject(r.Context(), h.bucket, model.ObjectPath)
	if err != nil {
		h.log.Errorw("models: GetObject failed",
			"name", name, "version", model.Version, "object", model.ObjectPath, "error", err)
		writeInternalError(w)
		return
	}
	defer func() { _ = rc.Close() }()

	// Large model artifact (hundreds of MiB) over a slow worker link can exceed
	// the global absolute WriteTimeout — clear the deadline before streaming.
	clearWriteDeadline(w, h.log, "models: clear write deadline failed", "name", name)

	// Set response headers.
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("X-Model-Checksum", model.Checksum)
	w.Header().Set("X-Model-Version", model.Version)
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, rc); err != nil {
		// Client likely disconnected mid-stream; log at warn to avoid noise.
		h.log.Warnw("models: copy to response body failed",
			"name", name, "version", model.Version, "error", err)
	}
}
