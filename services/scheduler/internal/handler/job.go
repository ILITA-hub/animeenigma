package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/service"
)

type JobHandler struct {
	jobService *service.JobService
	log        *logger.Logger
}

func NewJobHandler(jobService *service.JobService, log *logger.Logger) *JobHandler {
	return &JobHandler{
		jobService: jobService,
		log:        log,
	}
}

// GetJobStatus returns the status of all scheduled jobs
func (h *JobHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	status := h.jobService.GetStatus()
	httputil.OK(w, status)
}

// TriggerShikimoriSync manually triggers the Shikimori sync job
func (h *JobHandler) TriggerShikimoriSync(w http.ResponseWriter, r *http.Request) {
	go h.jobService.TriggerShikimoriSync(context.Background())
	httputil.OK(w, map[string]string{"status": "job triggered"})
}

// TriggerCleanup manually triggers the cleanup job
func (h *JobHandler) TriggerCleanup(w http.ResponseWriter, r *http.Request) {
	go h.jobService.TriggerCleanup(context.Background())
	httputil.OK(w, map[string]string{"status": "job triggered"})
}
