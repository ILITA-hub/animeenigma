// Package handler implements the HTTP surface for the storage service.
// Every route is Docker-network-only under /internal/storage/* (no gateway
// route) — see docs/superpowers/specs/2026-07-10-storage-service-design.md.
package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
)

// ingestExpirySeconds mirrors the 1h presigned-URL expiry used for every
// PUT/GET the backends hand out (service.Backends).
const ingestExpirySeconds = 3600

// Backends is the narrow surface StorageHandler depends on. The production
// implementation is service.Backends (real MinIO/S3 clients); unit tests
// inject a fake so no real MinIO is required.
type Backends interface {
	IngestURLs(ctx context.Context, storage, prefix string, files []string) ([]domain.PutURL, error)
	DownloadURLs(ctx context.Context, storage, prefix string) ([]domain.GetURL, error)
	Move(ctx context.Context, storage, fromPrefix, toPrefix string) (int, error)
	Copy(ctx context.Context, fromStorage, toStorage, prefix string) (int, int64, error)
	DeletePrefix(ctx context.Context, storage, prefix string) (int, error)
	List(ctx context.Context, storage, prefix string) ([]domain.Object, error)
	BaseURLs() map[string]string
	Health(ctx context.Context) map[string]string
}

// Placer is the narrow surface StorageHandler needs from service.Placement.
type Placer interface {
	Resolve(class, override string) (string, error)
}

// StorageHandler serves every /internal/storage/* route.
type StorageHandler struct {
	backends  Backends
	placement Placer
	log       *logger.Logger
}

// NewStorageHandler constructs a StorageHandler.
func NewStorageHandler(backends Backends, placement Placer, log *logger.Logger) *StorageHandler {
	return &StorageHandler{backends: backends, placement: placement, log: log}
}

// IngestURLs handles POST /internal/storage/ingest-urls. It resolves the
// destination backend via placement policy, then hands out one presigned
// PUT per requested file. Upload ordering (segments before playlist, etc.)
// is the caller's job, not this service's.
func (h *StorageHandler) IngestURLs(w http.ResponseWriter, r *http.Request) {
	var req domain.IngestURLsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	storage, err := h.placement.Resolve(req.Class, req.Override)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	urls, err := h.backends.IngestURLs(r.Context(), storage, req.Prefix, req.Files)
	if err != nil {
		h.logErr("ingest-urls", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.IngestURLsResponse{
		Storage:   storage,
		URLs:      urls,
		ExpiresIn: ingestExpirySeconds,
	})
}

// DownloadURLs handles POST /internal/storage/download-urls — presigned GET
// for every object under the given prefix.
func (h *StorageHandler) DownloadURLs(w http.ResponseWriter, r *http.Request) {
	var req domain.DownloadURLsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	urls, err := h.backends.DownloadURLs(r.Context(), req.Storage, req.Prefix)
	if err != nil {
		h.logErr("download-urls", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.DownloadURLsResponse{URLs: urls})
}

// Move handles POST /internal/storage/move — server-side prefix move within
// one backend (the pending/<job> -> linked flow).
func (h *StorageHandler) Move(w http.ResponseWriter, r *http.Request) {
	var req domain.MoveRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	moved, err := h.backends.Move(r.Context(), req.Storage, req.FromPrefix, req.ToPrefix)
	if err != nil {
		h.logErr("move", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.MoveResponse{Moved: moved})
}

// Copy handles POST /internal/storage/copy — cross-backend prefix copy
// (used by migration): GET-stream from the source client, PutObject to the
// target.
func (h *StorageHandler) Copy(w http.ResponseWriter, r *http.Request) {
	var req domain.CopyPrefixRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	copied, bytes, err := h.backends.Copy(r.Context(), req.FromStorage, req.ToStorage, req.Prefix)
	if err != nil {
		h.logErr("copy", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.CopyResponse{Copied: copied, Bytes: bytes})
}

// DeletePrefix handles DELETE /internal/storage/prefix — eviction/cleanup.
func (h *StorageHandler) DeletePrefix(w http.ResponseWriter, r *http.Request) {
	var req domain.DeletePrefixRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	deleted, err := h.backends.DeletePrefix(r.Context(), req.Storage, req.Prefix)
	if err != nil {
		h.logErr("delete-prefix", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.DeleteResponse{Deleted: deleted})
}

// List handles GET /internal/storage/list?storage=&prefix=.
func (h *StorageHandler) List(w http.ResponseWriter, r *http.Request) {
	storage := r.URL.Query().Get("storage")
	prefix := r.URL.Query().Get("prefix")
	if storage == "" {
		httputil.Error(w, errors.InvalidInput("storage query param is required"))
		return
	}

	objects, err := h.backends.List(r.Context(), storage, prefix)
	if err != nil {
		h.logErr("list", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, domain.ListResponse{Objects: objects})
}

// BaseURLs handles GET /internal/storage/base-urls — the canonical
// {backend_id: base_url} map (scheme from UseSSL); the only place that
// knows http://minio:9000/raw-library vs https://s3.firstvds.ru/raw-library.
func (h *StorageHandler) BaseURLs(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, h.backends.BaseURLs())
}

// Health handles GET /internal/storage/health — probes both backends via
// BucketExists. Always 200; callers decide what a "down" backend means for
// them.
func (h *StorageHandler) Health(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, h.backends.Health(r.Context()))
}

// logErr logs a failed backend operation at ERROR level — but only for
// genuine server-side failures (5xx: S3/MinIO calls that errored, backend
// misconfiguration). Client mistakes (4xx: unknown class/storage/override,
// caught as errors.AppError with a sub-500 StatusCode) are expected input
// validation, not operational failures, and skip logging entirely to keep
// error-level logs/alerts meaningful.
func (h *StorageHandler) logErr(op string, err error) {
	if h.log == nil {
		return
	}
	if appErr, ok := errors.IsAppError(err); ok && appErr.StatusCode < http.StatusInternalServerError {
		return
	}
	h.log.Errorw("storage operation failed", "op", op, "error", err)
}
