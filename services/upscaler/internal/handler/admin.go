package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/service"
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
	FindByJob(ctx context.Context, jobID string) ([]domain.UpscaleWorker, error)
}

// commander issues typed commands to workers via the control-plane hub.
type commander interface {
	Issue(workerID, cmd string, args json.RawMessage) error
}

// adminLogBuffer provides per-job log ring-buffer access and live subscriptions.
type adminLogBuffer interface {
	Tail(ctx context.Context, jobID string, n int) []service.LogLine
	Subscribe(jobID string) (<-chan service.LogLine, func())
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

	// Optional extensions wired via With* setters.
	cmd    commander
	logBuf adminLogBuffer
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
	// Reject an unknown status filter rather than silently returning an empty
	// list — an empty status (no filter) is allowed; a non-empty invalid one is a
	// client error.
	if f.Status != "" && !f.Status.IsValid() {
		httputil.BadRequest(w, "invalid status: "+string(f.Status))
		return
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
//
// Beyond flipping the DB status, CancelJob ALSO delivers a `cancel` command over
// the WS control plane to any worker currently processing the job, so a worker
// mid-segment aborts promptly instead of finishing then discovering the job is
// gone. The worker delivery is best-effort: a worker that is not connected (or a
// nil commander) does not fail the request — the DB cancel still applies, and the
// segment lease will expire on its own. (If no worker is bound to the job,
// DB-cancel is the entire effect.)
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

	// Best-effort: tell the worker(s) running this job to abort now.
	h.deliverCancelToWorkers(r.Context(), id)

	job.Status = domain.JobCancelled
	httputil.OK(w, job)
}

// deliverCancelToWorkers issues a `cancel` command over the WS control plane to
// every non-gone worker currently bound to jobID. It is best-effort and never
// affects the cancel request outcome: a nil commander, a worker-lookup error, or
// a not-connected worker are all logged and swallowed (the DB status is already
// cancelled, and the segment lease will expire regardless).
func (h *AdminHandler) deliverCancelToWorkers(ctx context.Context, jobID string) {
	if h.cmd == nil {
		return
	}
	workers, err := h.workers.FindByJob(ctx, jobID)
	if err != nil {
		h.log.Warnw("admin: cancel job — worker lookup failed (DB cancel still applied)", "job_id", jobID, "error", err)
		return
	}
	argsBytes, err := json.Marshal(map[string]string{"job_id": jobID})
	if err != nil {
		h.log.Warnw("admin: cancel job — marshal cancel args failed (best-effort)", "job_id", jobID, "error", err)
		return
	}
	args := json.RawMessage(argsBytes)
	for _, wk := range workers {
		if err := h.cmd.Issue(wk.WorkerID, "cancel", args); err != nil {
			h.log.Warnw("admin: cancel job — deliver cancel command failed (best-effort)",
				"job_id", jobID, "worker_id", wk.WorkerID, "error", err)
		}
	}
}

// RetryJob handles POST /api/upscale/jobs/{id}/retry.
// Re-queues a failed job (status → queued, error_text cleared).
// Returns 400 when the job is not in the "failed" state.
//
// NOTE: Retry does NOT reset segment state; retrying a finalization-failed job
// (all segments done) will re-finalize from stale state without re-upscaling.
// Phase 2 should add a segment reset before re-queuing.
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

// WithCommander wires a command issuer (e.g. *controlplane.Issuer) for
// PostWorkerCommand. Returns the handler for chaining.
func (h *AdminHandler) WithCommander(c commander) *AdminHandler {
	h.cmd = c
	return h
}

// WithLogBuffer wires a per-job log ring-buffer for GetJobLogs and StreamJobLogs.
// Returns the handler for chaining.
func (h *AdminHandler) WithLogBuffer(lb adminLogBuffer) *AdminHandler {
	h.logBuf = lb
	return h
}

// workerCommandRequest is the body for POST /api/upscale/workers/{id}/commands.
type workerCommandRequest struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

// PostWorkerCommand handles POST /api/upscale/workers/{id}/commands.
// Issues a typed command (cancel|drain|shutdown|reconfigure|update) to the named worker.
// Returns 400 for unknown/disallowed commands, 503 when the worker is not connected.
func (h *AdminHandler) PostWorkerCommand(w http.ResponseWriter, r *http.Request) {
	if h.cmd == nil {
		httputil.JSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "command issuer not configured"})
		return
	}
	workerID := chi.URLParam(r, "id")
	if strings.TrimSpace(workerID) == "" {
		httputil.BadRequest(w, "worker id is required")
		return
	}

	var req workerCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Cmd) == "" {
		httputil.BadRequest(w, "cmd is required")
		return
	}
	// Default args to empty object when omitted.
	if req.Args == nil {
		req.Args = json.RawMessage(`{}`)
	}

	if err := h.cmd.Issue(workerID, req.Cmd, req.Args); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not allowed") || strings.Contains(errMsg, "whitelist") {
			httputil.BadRequest(w, errMsg)
			return
		}
		// Worker not connected or send buffer full.
		h.log.Warnw("admin: issue command to worker failed", "worker_id", workerID, "cmd", req.Cmd, "error", err)
		httputil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": errMsg})
		return
	}
	httputil.OK(w, map[string]string{"status": "queued", "worker_id": workerID, "cmd": req.Cmd})
}

// GetJobLogs handles GET /api/upscale/jobs/{id}/logs.
// Returns the last N log lines from the ring-buffer (?n=<count>, default 100).
func (h *AdminHandler) GetJobLogs(w http.ResponseWriter, r *http.Request) {
	if h.logBuf == nil {
		httputil.JSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "log buffer not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	n := 100
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if v, err := strconv.Atoi(nStr); err == nil && v > 0 {
			n = v
		}
	}
	lines := h.logBuf.Tail(r.Context(), id, n)
	if lines == nil {
		lines = []service.LogLine{}
	}
	httputil.OK(w, lines)
}

// StreamJobLogs handles GET /api/upscale/jobs/{id}/logs/stream.
// Streams live log lines as SSE (text/event-stream). Closes when the client
// disconnects or the context is cancelled.
func (h *AdminHandler) StreamJobLogs(w http.ResponseWriter, r *http.Request) {
	if h.logBuf == nil {
		httputil.JSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "log buffer not configured"})
		return
	}
	id := chi.URLParam(r, "id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.JSON(w, http.StatusInternalServerError,
			map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Long-lived SSE log tail would hit the global absolute WriteTimeout no matter
	// how recently a line was written — clear the deadline.
	clearWriteDeadline(w, h.log, "upscaler: clear SSE write deadline failed", "job_id", id)

	// Subscribe BEFORE reading the Tail backlog so no line appended in the gap
	// between the two is lost. A duplicate of the most-recent line is acceptable;
	// dropping one is not.
	ch, cancel := h.logBuf.Subscribe(id)
	defer cancel()

	ctx := r.Context()
	// Replay recent history first so a client connecting mid-job sees context.
	for _, line := range h.logBuf.Tail(ctx, id, 100) {
		raw, err := json.Marshal(line)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			raw, err := json.Marshal(line)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", raw)
			flusher.Flush()
		}
	}
}
