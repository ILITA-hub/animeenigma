package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/jobs"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/service"
	"github.com/go-chi/chi/v5"
)

// TaskHandler handles task-related HTTP requests
type TaskHandler struct {
	exportService *service.ExportService
	loaderJob     *jobs.AnimeLoaderJob
	log           *logger.Logger
}

// NewTaskHandler creates a new task handler
func NewTaskHandler(
	exportService *service.ExportService,
	loaderJob *jobs.AnimeLoaderJob,
	log *logger.Logger,
) *TaskHandler {
	return &TaskHandler{
		exportService: exportService,
		loaderJob:     loaderJob,
		log:           log,
	}
}

// CreateExportJob creates a new MAL export job
func (h *TaskHandler) CreateExportJob(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateExportJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if req.UserID == "" || req.MALUsername == "" {
		httputil.BadRequest(w, "user_id and mal_username are required")
		return
	}

	job, err := h.exportService.CreateExportJob(r.Context(), &req)
	if err != nil {
		h.log.Errorw("failed to create export job",
			"user_id", req.UserID,
			"mal_username", req.MALUsername,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	// httputil.Created already wraps the payload in a top-level "data" key —
	// callers (player service) decode the job directly from there.
	httputil.Created(w, job.ToResponse())
}

// CreateTasks creates anime load tasks for an export job
func (h *TaskHandler) CreateTasks(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateTasksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if req.ExportJobID == "" || req.UserID == "" || len(req.Tasks) == 0 {
		httputil.BadRequest(w, "export_job_id, user_id, and tasks are required")
		return
	}

	if err := h.exportService.CreateTasks(r.Context(), &req); err != nil {
		h.log.Errorw("failed to create tasks",
			"export_job_id", req.ExportJobID,
			"task_count", len(req.Tasks),
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"message":    "tasks created",
		"task_count": len(req.Tasks),
	})
}

// GetExportJobStatus returns the status of an export job
func (h *TaskHandler) GetExportJobStatus(w http.ResponseWriter, r *http.Request) {
	exportID := chi.URLParam(r, "exportId")
	if exportID == "" {
		httputil.BadRequest(w, "export_id is required")
		return
	}

	job, err := h.exportService.GetExportJob(r.Context(), exportID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// Get detailed task stats
	stats, err := h.exportService.GetTaskStats(r.Context(), exportID)
	if err != nil {
		h.log.Warnw("failed to get task stats", "export_id", exportID, "error", err)
	}

	// Job fields at the top of "data" (what the player service decodes),
	// with detailed task stats embedded alongside.
	httputil.OK(w, struct {
		*domain.ExportJobResponse
		Stats *domain.TaskStats `json:"stats,omitempty"`
	}{job.ToResponse(), stats})
}

// CancelExportJob cancels an active export job (and drops its queued tasks)
func (h *TaskHandler) CancelExportJob(w http.ResponseWriter, r *http.Request) {
	exportID := chi.URLParam(r, "exportId")
	if exportID == "" {
		httputil.BadRequest(w, "export_id is required")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if req.UserID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}

	if err := h.exportService.CancelExport(r.Context(), req.UserID, exportID); err != nil {
		h.log.Warnw("failed to cancel export job",
			"export_id", exportID,
			"user_id", req.UserID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"message": "export cancelled"})
}

// ListUserExportJobs returns all export jobs for a user
func (h *TaskHandler) ListUserExportJobs(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}

	jobs, err := h.exportService.GetUserExports(r.Context(), userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	responses := make([]*domain.ExportJobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, job.ToResponse())
	}

	httputil.OK(w, responses)
}

// GetWorkerStatus returns the status of the anime loader worker
func (h *TaskHandler) GetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	status := h.loaderJob.GetStatus()
	httputil.OK(w, status)
}

// DeletePendingTask removes a task from the queue (for "Load Now" feature)
func (h *TaskHandler) DeletePendingTask(w http.ResponseWriter, r *http.Request) {
	malIDStr := chi.URLParam(r, "malId")
	if malIDStr == "" {
		httputil.BadRequest(w, "mal_id is required")
		return
	}

	var malID int
	if _, err := json.Marshal(malIDStr); err != nil {
		httputil.BadRequest(w, "invalid mal_id")
		return
	}
	// Parse malID from string
	if n, err := json.Number(malIDStr).Int64(); err == nil {
		malID = int(n)
	} else {
		httputil.BadRequest(w, "invalid mal_id")
		return
	}

	if err := h.exportService.DeletePendingTask(r.Context(), malID); err != nil {
		h.log.Warnw("failed to delete pending task", "mal_id", malID, "error", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"message": "task removed from queue"})
}
