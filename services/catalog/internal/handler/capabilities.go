package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
)

type capabilityService interface {
	Report(ctx context.Context, animeID string) (domain.CapabilityReport, error)
}

// CapabilitiesHandler serves GET /api/anime/{animeId}/capabilities.
type CapabilitiesHandler struct {
	svc capabilityService
	log *logger.Logger
}

// NewCapabilitiesHandler constructs the handler. log may be nil.
func NewCapabilitiesHandler(svc capabilityService, log *logger.Logger) *CapabilitiesHandler {
	return &CapabilitiesHandler{svc: svc, log: log}
}

// Get handles GET /api/anime/{animeId}/capabilities.
// Returns a ranked capability report (EN family in P4a) for the given anime.
func (h *CapabilitiesHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	report, err := h.svc.Report(r.Context(), animeID)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("capabilities assemble failed", "anime_id", animeID, "error", err)
		}
		httputil.Error(w, errors.Internal("failed to assemble capabilities"))
		return
	}
	httputil.OK(w, report)
}
