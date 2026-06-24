package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// adminJobRepo is the minimal JobRepository surface needed by AdminHandler.
type adminJobRepo interface {
	Create(ctx context.Context, job *domain.UpscaleJob) error
	Get(ctx context.Context, id string) (*domain.UpscaleJob, error)
	List(ctx context.Context, f repo.JobFilter) ([]domain.UpscaleJob, error)
	UpdateStatus(ctx context.Context, id string, status domain.JobStatus, errText string) error
}

// adminWorkerRepo is the minimal WorkerRepository surface needed by AdminHandler.
type adminWorkerRepo interface {
	ListConnected(ctx context.Context, since time.Time) ([]domain.UpscaleWorker, error)
}

// AdminHandler implements the /api/upscale/* admin REST API.
//
// Security note: these endpoints are only reachable via requireGatewayInternal
// middleware (which checks for the X-Gateway-Internal header injected by the
// gateway's admin-gated proxy). Input validation on error returns 400 with
// detail — fine for admin-facing endpoints.
type AdminHandler struct {
	jobs         adminJobRepo
	workers      adminWorkerRepo
	defaultScale int
	defaultModel string
	log          *logger.Logger
}

// NewAdminHandler constructs an AdminHandler. defaultScale comes from
// config.Upscaler.DefaultScale (env DEFAULT_SCALE, default 2). defaultModel
// is an optional override; if blank the caller must supply "model" in POST
// /jobs bodies.
func NewAdminHandler(
	jobs adminJobRepo,
	workers adminWorkerRepo,
	defaultScale int,
	defaultModel string,
	log *logger.Logger,
) *AdminHandler {
	if log == nil {
		log = logger.Default()
	}
	if defaultScale <= 0 {
		defaultScale = 2
	}
	return &AdminHandler{
		jobs:         jobs,
		workers:      workers,
		defaultScale: defaultScale,
		defaultModel: defaultModel,
		log:          log,
	}
}

// createJobRequest is the POST /api/upscale/jobs body.
type createJobRequest struct {
	ShikimoriID     string `json:"shikimori_id"`
	Episode         int    `json:"episode"`
	Model           string `json:"model"`
	Scale           int    `json:"scale"`
	LibraryInfohash string `json:"library_infohash,omitempty"`
}

func (r *createJobRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.ShikimoriID) == "" {
		errs = append(errs, "shikimori_id is required")
	}
	if r.Episode < 1 {
		errs = append(errs, "episode must be >= 1")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// CreateJob handles POST /api/upscale/jobs.
// Creates a queued UpscaleJob. model defaults to AdminHandler.defaultModel (if
// set); scale defaults to AdminHandler.defaultScale when omitted or zero.
func (h *AdminHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := httputil.BindAndValidate(r, &req); err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}

	// Apply defaults.
	if req.Model == "" {
		if h.defaultModel == "" {
			httputil.BadRequest(w, "model is required")
			return
		}
		req.Model = h.defaultModel
	}
	if req.Scale <= 0 {
		req.Scale = h.defaultScale
	}

	job := &domain.UpscaleJob{
		ShikimoriID:     strings.TrimSpace(req.ShikimoriID),
		Episode:         req.Episode,
		Model:           req.Model,
		Scale:           req.Scale,
		LibraryInfohash: req.LibraryInfohash,
		Status:          domain.JobQueued,
	}
	if err := h.jobs.Create(r.Context(), job); err != nil {
		h.log.Errorw("admin: create job failed", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.Created(w, job)
}

// ListJobs handles GET /api/upscale/jobs.
// Supports ?status=<status>, ?shikimori_id=<id>, ?limit=<n>, ?offset=<n>.
func (h *AdminHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repo.JobFilter{
		Status:      domain.JobStatus(q.Get("status")),
		ShikimoriID: q.Get("shikimori_id"),
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			f.Limit = v
		}
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			f.Offset = v
		}
	}

	jobs, err := h.jobs.List(r.Context(), f)
	if err != nil {
		h.log.Errorw("admin: list jobs failed", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, jobs)
}

// GetJob handles GET /api/upscale/jobs/{id}.
// Returns 404 if the job does not exist.
func (h *AdminHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.jobs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(w, "job")
			return
		}
		h.log.Errorw("admin: get job failed", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, job)
}

// CancelJob handles POST /api/upscale/jobs/{id}/cancel.
// Sets status to "cancelled" if the job is not already in a terminal state.
// Returns 409 when already terminal.
func (h *AdminHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.jobs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(w, "job")
			return
		}
		h.log.Errorw("admin: cancel job — get failed", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	if job.Status.IsTerminal() {
		// Already in a terminal state — cancellation has no effect.
		// Return 409 Conflict so the caller knows the action was a no-op.
		httputil.JSON(w, http.StatusConflict,
			map[string]string{"error": "job is already in a terminal state", "status": string(job.Status)})
		return
	}
	if err := h.jobs.UpdateStatus(r.Context(), id, domain.JobCancelled, ""); err != nil {
		h.log.Errorw("admin: cancel job — update failed", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	job.Status = domain.JobCancelled
	httputil.OK(w, job)
}

// RetryJob handles POST /api/upscale/jobs/{id}/retry.
// Re-queues a failed job (status → queued, error_text cleared).
// Returns 400 when the job is not in the "failed" state.
func (h *AdminHandler) RetryJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.jobs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(w, "job")
			return
		}
		h.log.Errorw("admin: retry job — get failed", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	if job.Status != domain.JobFailed {
		httputil.BadRequest(w, "only failed jobs can be retried (current status: "+string(job.Status)+")")
		return
	}
	if err := h.jobs.UpdateStatus(r.Context(), id, domain.JobQueued, ""); err != nil {
		h.log.Errorw("admin: retry job — update failed", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	job.Status = domain.JobQueued
	job.ErrorText = ""
	httputil.OK(w, job)
}

// ListWorkers handles GET /api/upscale/workers.
// Returns all workers that sent a heartbeat in the last 5 minutes and are not
// marked "gone". The 5-minute window mirrors the controlplane heartbeat TTL.
func (h *AdminHandler) ListWorkers(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-5 * time.Minute)
	workers, err := h.workers.ListConnected(r.Context(), since)
	if err != nil {
		h.log.Errorw("admin: list workers failed", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, workers)
}
