package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// MALExportHandler handles MAL export HTTP requests
type MALExportHandler struct {
	exportService *service.MALExportService
	log           *logger.Logger
}

// NewMALExportHandler creates a new MAL export handler
func NewMALExportHandler(exportService *service.MALExportService, log *logger.Logger) *MALExportHandler {
	return &MALExportHandler{
		exportService: exportService,
		log:           log,
	}
}

// InitiateExportRequest is the request to start a MAL export
type InitiateExportRequest struct {
	MALUsername string `json:"mal_username"`
}

// InitiateExport starts a new MAL export job
func (h *MALExportHandler) InitiateExport(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req InitiateExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if req.MALUsername == "" {
		httputil.BadRequest(w, "mal_username is required")
		return
	}

	h.log.Infow("initiating MAL export",
		"user_id", claims.UserID,
		"mal_username", req.MALUsername,
	)

	job, err := h.exportService.InitiateExport(r.Context(), claims.UserID, req.MALUsername)
	if err != nil {
		h.log.Errorw("failed to initiate export",
			"user_id", claims.UserID,
			"mal_username", req.MALUsername,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	h.log.Infow("MAL export initiated",
		"user_id", claims.UserID,
		"export_id", job.ID,
		"total_anime", job.TotalAnime,
	)

	httputil.Created(w, map[string]interface{}{
		"data": job,
	})
}

// GetExportStatus returns the status of an export job
func (h *MALExportHandler) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	exportID := chi.URLParam(r, "exportId")
	if exportID == "" {
		httputil.BadRequest(w, "export_id is required")
		return
	}

	job, err := h.exportService.GetExportStatus(r.Context(), exportID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"data": job,
	})
}

// GetUserExports returns all export jobs for the current user
func (h *MALExportHandler) GetUserExports(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	exports, err := h.exportService.GetUserExports(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"data": exports,
	})
}

// CancelExport cancels an active export job
func (h *MALExportHandler) CancelExport(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	exportID := chi.URLParam(r, "exportId")
	if exportID == "" {
		httputil.BadRequest(w, "export_id is required")
		return
	}

	if err := h.exportService.CancelExport(r.Context(), claims.UserID, exportID); err != nil {
		h.log.Warnw("failed to cancel export",
			"user_id", claims.UserID,
			"export_id", exportID,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	h.log.Infow("MAL export cancelled",
		"user_id", claims.UserID,
		"export_id", exportID,
	)

	httputil.OK(w, map[string]string{"message": "export cancelled"})
}
