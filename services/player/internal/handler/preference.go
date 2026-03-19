package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type PreferenceHandler struct {
	prefService *service.PreferenceService
	log         *logger.Logger
}

func NewPreferenceHandler(prefService *service.PreferenceService, log *logger.Logger) *PreferenceHandler {
	return &PreferenceHandler{prefService: prefService, log: log}
}

// ResolvePreference resolves the best watch combo for a user and anime
func (h *PreferenceHandler) ResolvePreference(w http.ResponseWriter, r *http.Request) {
	var req domain.ResolveRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.AnimeID == "" {
		httputil.Error(w, errors.InvalidInput("anime_id is required"))
		return
	}
	if len(req.Available) == 0 {
		httputil.Error(w, errors.InvalidInput("available combos must not be empty"))
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	resp, err := h.prefService.Resolve(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, resp)
}

// GetAnimePreference returns the user's saved preference for a specific anime
func (h *PreferenceHandler) GetAnimePreference(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	pref, err := h.prefService.GetAnimePreference(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, errors.NotFound("anime preference"))
		return
	}

	httputil.OK(w, pref)
}

// GetGlobalPreferences returns the user's top combos ranked by watch count
func (h *PreferenceHandler) GetGlobalPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	combos, err := h.prefService.GetGlobalPreferences(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if combos == nil {
		combos = []domain.ComboCount{}
	}

	httputil.OK(w, map[string]interface{}{
		"top_combos": combos,
	})
}
