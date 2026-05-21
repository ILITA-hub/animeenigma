package handler

// Phase 2 v1.0 Notifications Engine — admin trigger handlers (D-DET-05 /
// D-DET-06). NO middleware: same gateway-non-routing model as Phase 1's
// /internal/notifications. Reachable only from inside the Docker network.
//
// POST /internal/detector/run-once  — synchronous detector run, returns
//                                     a RunReport JSON
// POST /internal/cleanup/run-once   — synchronous cleanup run, returns
//                                     {"deleted": N}
//
// Both endpoints exist primarily so SC2..SC7 in ROADMAP can be verified
// deterministically without waiting for the hourly / 03:30 ticks. The
// `make run-detector-once` / `make run-cleanup-once` Makefile targets are
// thin shells over `docker compose exec`.

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/job"
)

// AdminHandler exposes the manual-trigger endpoints for the Phase 2
// detector + cleanup jobs.
type AdminHandler struct {
	detector *job.NewEpisodeDetectorJob
	cleanup  *job.DismissedRetentionCleanupJob
	log      *logger.Logger
}

// NewAdminHandler constructs the handler.
func NewAdminHandler(
	detector *job.NewEpisodeDetectorJob,
	cleanup *job.DismissedRetentionCleanupJob,
	log *logger.Logger,
) *AdminHandler {
	return &AdminHandler{detector: detector, cleanup: cleanup, log: log}
}

// RunDetectorOnce fires the detector synchronously and returns the
// RunReport JSON. Blocks until the run completes — convenient for
// `make run-detector-once` so the make-target exit code mirrors the run
// outcome.
func (h *AdminHandler) RunDetectorOnce(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		httputil.Error(w, &noDetectorErr{})
		return
	}
	report, err := h.detector.Run(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, report)
}

// RunCleanupOnce fires the cleanup synchronously and returns the deleted
// count.
func (h *AdminHandler) RunCleanupOnce(w http.ResponseWriter, r *http.Request) {
	if h.cleanup == nil {
		httputil.Error(w, &noCleanupErr{})
		return
	}
	deleted, err := h.cleanup.Run(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"deleted": deleted})
}

// noDetectorErr / noCleanupErr — when scheduler is disabled
// (NOTIFICATIONS_DETECTOR_ENABLED=false) the admin handler still mounts
// but the jobs aren't wired. Return 503 so callers know it's an explicit
// disable, not a missing route.
type noDetectorErr struct{}

func (e *noDetectorErr) Error() string { return "detector disabled" }

type noCleanupErr struct{}

func (e *noCleanupErr) Error() string { return "cleanup disabled" }
