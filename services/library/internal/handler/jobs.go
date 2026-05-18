package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/go-chi/chi/v5"
)

// JobStoreAPI is the slice of repo.JobRepository the handler needs.
// Defined here so jobs_test.go can inject a stub without spinning
// up a real Postgres.
type JobStoreAPI interface {
	Create(ctx context.Context, job *domain.Job) error
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error)
}

// DiskGuardAPI is the slice of service.DiskGuard the handler needs.
type DiskGuardAPI interface {
	Allow(minFreePct int) (allowed bool, freePct int, err error)
}

// JobCanceller is the slice of service.WorkerPool the handler needs.
// CancelJob flips status FIRST then signals the in-memory handle —
// callers don't have to know that.
type JobCanceller interface {
	CancelJob(ctx context.Context, jobID string) error
}

// JobsHandler implements the admin-gated CRUD on /api/library/jobs.
// Auth is enforced at the gateway (gateway routes /api/library/*
// behind JWT + AdminMiddleware); the handler trusts what the gateway
// forwards.
type JobsHandler struct {
	jobRepo    JobStoreAPI
	diskGuard  DiskGuardAPI
	canceller  JobCanceller
	metrics    *metrics.LibraryMetrics
	minFreePct int
	log        *logger.Logger
}

// NewJobsHandler constructs a JobsHandler.
func NewJobsHandler(
	jobRepo JobStoreAPI,
	diskGuard DiskGuardAPI,
	canceller JobCanceller,
	libMetrics *metrics.LibraryMetrics,
	minFreePct int,
	log *logger.Logger,
) *JobsHandler {
	return &JobsHandler{
		jobRepo:    jobRepo,
		diskGuard:  diskGuard,
		canceller:  canceller,
		metrics:    libMetrics,
		minFreePct: minFreePct,
		log:        log,
	}
}

// createJobRequest is the POST body shape locked in 03-SPEC.md.
// Server fills id / status / timestamps via DB defaults.
type createJobRequest struct {
	Magnet      string `json:"magnet"`
	Title       string `json:"title"`
	Source      string `json:"source"`
	Uploader    string `json:"uploader,omitempty"`
	Quality     string `json:"quality,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	ShikimoriID string `json:"shikimori_id,omitempty"`
}

var allowedSources = map[domain.JobSource]bool{
	domain.JobSourceNyaa:       true,
	domain.JobSourceAnimeTosho: true,
	domain.JobSourceManual:     true,
}

var allowedStatuses = map[domain.JobStatus]bool{
	domain.JobStatusQueued:      true,
	domain.JobStatusDownloading: true,
	domain.JobStatusEncoding:    true,
	domain.JobStatusUploading:   true,
	domain.JobStatusDone:        true,
	domain.JobStatusFailed:      true,
	domain.JobStatusCancelled:   true,
}

// Create handles POST /api/library/jobs.
//
// Validation order matters: cheap checks (body parse + required
// fields + source enum) before the expensive magnet parse, and the
// disk guard last so a 400 doesn't burn a Statfs syscall.
func (h *JobsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createJobRequest
	if err := httputil.Bind(r, &body); err != nil {
		httputil.Error(w, err)
		return
	}
	body.Magnet = strings.TrimSpace(body.Magnet)
	body.Title = strings.TrimSpace(body.Title)
	body.Source = strings.TrimSpace(body.Source)

	if body.Title == "" {
		httputil.BadRequest(w, "title is required")
		return
	}
	if body.Magnet == "" {
		httputil.BadRequest(w, "magnet is required")
		return
	}
	src := domain.JobSource(body.Source)
	if !allowedSources[src] {
		httputil.BadRequest(w, "source must be one of: nyaa, animetosho, manual")
		return
	}
	if _, err := metainfo.ParseMagnetUri(body.Magnet); err != nil {
		if h.metrics != nil {
			h.metrics.IncEnqueueRejected("invalid_magnet")
		}
		httputil.BadRequest(w, "invalid magnet")
		return
	}

	if h.diskGuard != nil {
		allowed, freePct, err := h.diskGuard.Allow(h.minFreePct)
		if err != nil {
			if h.log != nil {
				h.log.Warnw("disk guard check failed", "error", err)
			}
			// Don't 500 — log and let the worker handle it on dequeue.
		} else if !allowed {
			if h.metrics != nil {
				h.metrics.IncEnqueueRejected("disk_full")
			}
			if h.log != nil {
				h.log.Warnw("enqueue rejected: disk full",
					"free_pct", freePct, "min_required_pct", h.minFreePct,
				)
			}
			writeInsufficientStorage(w)
			return
		}
	}

	job := &domain.Job{
		Source:      src,
		Magnet:      body.Magnet,
		Title:       body.Title,
		Uploader:    body.Uploader,
		Quality:     body.Quality,
		SizeBytes:   body.SizeBytes,
		ShikimoriID: body.ShikimoriID,
		Status:      domain.JobStatusQueued,
	}
	if err := h.jobRepo.Create(r.Context(), job); err != nil {
		httputil.Error(w, err)
		return
	}
	if h.metrics != nil {
		h.metrics.IncJobsTotal(string(domain.JobStatusQueued))
	}
	httputil.Created(w, job)
}

// List handles GET /api/library/jobs?status=&limit=.
//
// status is comma-separated (e.g. "queued,downloading"). Unknown
// values → 400. limit defaults to 100, clamps 1..500.
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	statuses, err := parseStatusList(r.URL.Query().Get("status"))
	if err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"))

	jobs, err := h.jobRepo.List(r.Context(), repo.JobFilter{
		Statuses: statuses,
		Limit:    limit,
	})
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if jobs == nil {
		jobs = []domain.Job{}
	}
	httputil.OK(w, map[string]any{"jobs": jobs})
}

// Get handles GET /api/library/jobs/{id}.
func (h *JobsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	job, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, job)
}

// Delete handles DELETE /api/library/jobs/{id}.
//
// canceller.CancelJob flips status='cancelled' first, then signals
// the in-memory handle.Cancel() — the CONTEXT-locked order that keeps
// status consistent even if the in-memory cancel is lost.
func (h *JobsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	if h.canceller == nil {
		httputil.Error(w, liberrors.Internal("canceller not configured"))
		return
	}
	if err := h.canceller.CancelJob(r.Context(), id); err != nil {
		httputil.Error(w, err)
		return
	}
	if h.metrics != nil {
		h.metrics.IncJobsTotal(string(domain.JobStatusCancelled))
	}
	httputil.NoContent(w)
}

// parseStatusList — comma-separated, validated against the SQL enum.
func parseStatusList(raw string) ([]domain.JobStatus, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]domain.JobStatus, 0, len(parts))
	for _, p := range parts {
		s := domain.JobStatus(strings.TrimSpace(p))
		if !allowedStatuses[s] {
			return nil, &badStatusError{value: string(s)}
		}
		out = append(out, s)
	}
	return out, nil
}

type badStatusError struct{ value string }

func (e *badStatusError) Error() string { return "unknown status: " + e.value }

func parseLimit(raw string) int {
	limit := 100
	if raw == "" {
		return limit
	}
	if n, err := strconv.Atoi(raw); err == nil {
		limit = n
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

// writeInsufficientStorage emits HTTP 507 with the SPEC-locked body.
// httputil has no helper for 507 so we drive httputil.JSON directly.
func writeInsufficientStorage(w http.ResponseWriter) {
	httputil.JSON(w, http.StatusInsufficientStorage, map[string]string{"error": "disk_full"})
}
