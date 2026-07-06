package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// SecretFeatureHandler exposes the admin management + public enforcement surface
// for the footer «Секретная фича» roulette.
type SecretFeatureHandler struct {
	svc *service.SecretFeatureService
	log *logger.Logger
}

func NewSecretFeatureHandler(svc *service.SecretFeatureService, log *logger.Logger) *SecretFeatureHandler {
	return &SecretFeatureHandler{svc: svc, log: log}
}

// GetConfig — GET /api/admin/secret-features (admin).
func (h *SecretFeatureHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.GetConfig(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, cfg)
}

// SetRoulette — PUT /api/admin/secret-features/roulette (admin).
func (h *SecretFeatureHandler) SetRoulette(w http.ResponseWriter, r *http.Request) {
	var req domain.SetSecretFeatureFlagRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.svc.SetRoulette(r.Context(), req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	h.log.Infow("secret-feature roulette toggled", "enabled", req.Enabled)
	h.respondConfig(w, r)
}

// SetFeature — PUT /api/admin/secret-features/feature/{key} (admin).
func (h *SecretFeatureHandler) SetFeature(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		httputil.BadRequest(w, "key is required")
		return
	}
	var req domain.SetSecretFeatureFlagRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.svc.SetFeature(r.Context(), key, req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	h.log.Infow("secret-feature toggled", "key", key, "enabled", req.Enabled)
	h.respondConfig(w, r)
}

// PublicState — GET /api/secret-features/state (public, anonymous). The footer
// roulette reads this to enforce admin toggles; callers fail open on error.
func (h *SecretFeatureHandler) PublicState(w http.ResponseWriter, r *http.Request) {
	state, err := h.svc.PublicState(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, state)
}

// respondConfig returns the freshly-resolved admin config after a mutation so
// the admin UI can update without a second round-trip.
func (h *SecretFeatureHandler) respondConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.GetConfig(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, cfg)
}
