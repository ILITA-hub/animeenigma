package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type CompatibilityHandler struct {
	svc *service.CompatibilityService
	log *logger.Logger
}

func NewCompatibilityHandler(s *service.CompatibilityService, log *logger.Logger) *CompatibilityHandler {
	return &CompatibilityHandler{svc: s, log: log}
}

// GetCompatibility handles GET /api/users/{userId}/compatibility.
// JWT required; computes the viewer (claims) vs {userId} (the profile owner).
func (h *CompatibilityHandler) GetCompatibility(w http.ResponseWriter, r *http.Request) {
	ownerID := chi.URLParam(r, "userId")
	if ownerID == "" {
		httputil.BadRequest(w, "userId is required")
		return
	}
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	if claims.UserID == ownerID {
		// own profile: compatibility is meaningless — return 100% / no sample so the FE can hide it
		httputil.OK(w, map[string]any{"percent": 100, "shared_count": 0, "shared_sample": []string{}, "self": true})
		return
	}
	res, err := h.svc.Compute(r.Context(), claims.UserID, ownerID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, res)
}
