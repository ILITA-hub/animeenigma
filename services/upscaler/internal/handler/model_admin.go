package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// modelRepo is the minimal interface ModelAdminHandler needs from the model repository.
type modelRepo interface {
	Upsert(ctx context.Context, m *domain.UpscaleModel) error
	Get(ctx context.Context, name, version string) (*domain.UpscaleModel, error)
	List(ctx context.Context) ([]domain.UpscaleModel, error)
}

// modelUploader is the minimal MinIO interface needed for uploading model artifacts.
// It matches the Uploader interface in the minio package, but is declared here so
// ModelAdminHandler does not import the minio package directly (test seam).
type modelUploader interface {
	PutObject(ctx context.Context, bucket, object string, reader interface {
		Read(p []byte) (int, error)
	}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
}

// ModelAdminHandler implements the admin model registry endpoints:
//
//	POST /api/upscale/models       — upload artifact, register metadata
//	GET  /api/upscale/models       — list all registered models
//	GET  /api/upscale/models/{name} — get latest/all versions for a model name
//
// All endpoints are gated by requireGatewayInternal (X-Gateway-Internal header)
// via the router group; no additional auth is needed here.
type ModelAdminHandler struct {
	repo     modelRepo
	uploader modelUploader
	bucket   string
	log      *logger.Logger
}

// NewModelAdminHandler constructs a ModelAdminHandler.
// uploader is typically the *minio.Writer's underlying Uploader (the Writer
// itself satisfies modelUploader via its PutObject method).
// bucket is the MinIO bucket to use (e.g. cfg.Upscaler.MinIO.Bucket).
func NewModelAdminHandler(repo modelRepo, uploader modelUploader, bucket string, log *logger.Logger) *ModelAdminHandler {
	if log == nil {
		log = logger.Default()
	}
	if bucket == "" {
		bucket = "raw-library"
	}
	return &ModelAdminHandler{repo: repo, uploader: uploader, bucket: bucket, log: log}
}

// isNameSafe returns true if s contains no path-traversal characters.
// name and version form the MinIO object key `models/{name}/{version}.tar`,
// so neither may contain '/' or '..'.
func isNameSafe(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") || strings.Contains(s, "..") || strings.Contains(s, "\\") {
		return false
	}
	return true
}

// UploadModel handles POST /api/upscale/models.
//
// Multipart fields:
//   - name    (required): model name, must be path-safe.
//   - version (optional, default "1"): model version, must be path-safe.
//   - scale   (optional, informational): upscale factor.
//   - artifact (file, required): opaque TAR of model weight files.
//
// The artifact is streamed to MinIO at key `models/{name}/{version}.tar` while
// a SHA-256 hash is computed via io.TeeReader (no full-file buffering). The
// computed checksum and object key are stored in the upscale_models table.
// Returns 201 with the stored UpscaleModel metadata.
func (h *ModelAdminHandler) UploadModel(w http.ResponseWriter, r *http.Request) {
	// 32 MiB header memory limit; body is streamed, not buffered.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httputil.BadRequest(w, "invalid multipart form: "+err.Error())
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if !isNameSafe(name) {
		httputil.BadRequest(w, "name is required and must not contain '/', '..', or '\\'")
		return
	}

	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		version = "1"
	}
	if !isNameSafe(version) {
		httputil.BadRequest(w, "version must not contain '/', '..', or '\\'")
		return
	}

	file, hdr, err := r.FormFile("artifact")
	if err != nil {
		httputil.BadRequest(w, "artifact file is required: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	if hdr.Size == 0 {
		httputil.BadRequest(w, "artifact must not be empty")
		return
	}

	// Object key: models/{name}/{version}.tar
	objectKey := fmt.Sprintf("models/%s/%s.tar", name, version)

	// Stream artifact → MinIO while computing SHA-256 via TeeReader.
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)

	_, err = h.uploader.PutObject(r.Context(), h.bucket, objectKey, tee, hdr.Size, minio.PutObjectOptions{
		ContentType: "application/x-tar",
	})
	if err != nil {
		h.log.Errorw("model admin: PutObject failed",
			"name", name, "version", version, "object", objectKey, "error", err)
		httputil.Error(w, fmt.Errorf("store model artifact: %w", err))
		return
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	m := &domain.UpscaleModel{
		Name:       name,
		Version:    version,
		Checksum:   checksum,
		ObjectPath: objectKey,
		Builtin:    false,
	}
	if err := h.repo.Upsert(r.Context(), m); err != nil {
		h.log.Errorw("model admin: Upsert failed",
			"name", name, "version", version, "error", err)
		httputil.Error(w, fmt.Errorf("register model: %w", err))
		return
	}

	httputil.Created(w, m)
}

// ListModels handles GET /api/upscale/models.
// Returns all registered models ordered by name, version.
func (h *ModelAdminHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.repo.List(r.Context())
	if err != nil {
		h.log.Errorw("model admin: List failed", "error", err)
		httputil.Error(w, err)
		return
	}
	if models == nil {
		models = []domain.UpscaleModel{}
	}
	httputil.OK(w, models)
}

// GetModelByName handles GET /api/upscale/models/{name}.
// Returns all registered versions for the named model (ordered by version).
// Returns 404 when no rows exist for the given name.
//
// The optional ?version= query param narrows to a single row.
func (h *ModelAdminHandler) GetModelByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !isNameSafe(name) {
		httputil.BadRequest(w, "invalid model name")
		return
	}

	version := r.URL.Query().Get("version")

	if version != "" {
		// Single-row lookup.
		m, err := h.repo.Get(r.Context(), name, version)
		if err != nil {
			if isNotFound(err) {
				httputil.NotFound(w, "model")
				return
			}
			h.log.Errorw("model admin: Get failed", "name", name, "version", version, "error", err)
			httputil.Error(w, err)
			return
		}
		httputil.OK(w, m)
		return
	}

	// All versions for this name — use List + filter (no per-name list method;
	// List returns ALL rows ordered by name,version which is fast for small
	// model registries and avoids adding a new repo method for T26).
	all, err := h.repo.List(r.Context())
	if err != nil {
		h.log.Errorw("model admin: List (by name) failed", "name", name, "error", err)
		httputil.Error(w, err)
		return
	}
	var rows []domain.UpscaleModel
	for _, m := range all {
		if m.Name == name {
			rows = append(rows, m)
		}
	}
	if len(rows) == 0 {
		httputil.NotFound(w, "model")
		return
	}
	httputil.OK(w, rows)
}

// isNotFound reports whether err signals a "record not found" condition
// (gorm.ErrRecordNotFound or any error whose message matches the common text).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Import-free check: gorm.ErrRecordNotFound.Error() == "record not found"
	return err == gorm.ErrRecordNotFound || strings.Contains(err.Error(), "record not found")
}
