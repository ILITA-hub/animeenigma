package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
)

// capabilitiesCtxTimeout bounds the capability fan-out (service/capability
// buildFamilies spins up 4 goroutines: BuildENFamily + kodik/animelib/hanime).
// Without it, a stuck upstream leg — notably the Kodik parser, whose
// GetTranslations relies only on its 30s http.Client timeout — would hang the
// request up to the server WriteTimeout. Mirrors spotlight.go's
// spotlightCtxTimeout (handler/spotlight.go).
const capabilitiesCtxTimeout = 3 * time.Second

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
	ctx, cancel := context.WithTimeout(r.Context(), capabilitiesCtxTimeout)
	defer cancel()
	report, err := h.svc.Report(ctx, animeID)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("capabilities assemble failed", "anime_id", animeID, "error", err)
		}
		httputil.Error(w, errors.Internal("failed to assemble capabilities"))
		return
	}
	httputil.OK(w, report)
}
