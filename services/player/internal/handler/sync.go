package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/go-chi/chi/v5"
)

type SyncHandler struct {
	syncRepo *repo.SyncRepository
	log      *logger.Logger
}

func NewSyncHandler(syncRepo *repo.SyncRepository, log *logger.Logger) *SyncHandler {
	return &SyncHandler{
		syncRepo: syncRepo,
		log:      log,
	}
}

// sourceStatus holds the active and last completed sync job for a source.
type sourceStatus struct {
	Active   *domain.SyncJob `json:"active"`
	LastSync *domain.SyncJob `json:"last_sync"`
}

// GetJobStatus returns the status of a specific import job.
// GET /api/users/import/{jobId}
func (h *SyncHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		httputil.BadRequest(w, "job ID is required")
		return
	}

	job, err := h.syncRepo.GetByID(r.Context(), jobID)
	if err != nil {
		h.log.Errorw("failed to get sync job",
			"job_id", jobID,
			"user_id", claims.UserID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	if job == nil || job.UserID != claims.UserID {
		httputil.NotFound(w, "import job")
		return
	}

	httputil.OK(w, job)
}

// GetSyncStatus returns the active and last completed sync job for each source.
// GET /api/users/sync/status
func (h *SyncHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	ctx := r.Context()
	userID := claims.UserID
	sources := []string{"mal", "shikimori"}

	result := make(map[string]sourceStatus, len(sources))

	for _, source := range sources {
		active, err := h.syncRepo.GetActiveByUserAndSource(ctx, userID, source)
		if err != nil {
			h.log.Errorw("failed to get active sync job",
				"user_id", userID,
				"source", source,
				"error", err,
			)
			httputil.Error(w, err)
			return
		}

		lastSync, err := h.syncRepo.GetLatestByUserAndSource(ctx, userID, source)
		if err != nil {
			h.log.Errorw("failed to get latest sync job",
				"user_id", userID,
				"source", source,
				"error", err,
			)
			httputil.Error(w, err)
			return
		}

		result[source] = sourceStatus{
			Active:   active,
			LastSync: lastSync,
		}
	}

	httputil.OK(w, result)
}
