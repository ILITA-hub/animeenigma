package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// writePrefsVersionHeader sets X-Prefs-Version on the response so the frontend
// useUserPreferences composable can detect cross-device prefs changes and
// invalidate its 24h cache. Best-effort: failures are logged not surfaced.
func (h *PreferenceHandler) writePrefsVersionHeader(w http.ResponseWriter, r *http.Request, userID string) {
	if userID == "" {
		return
	}
	v, err := h.prefService.GetPrefsVersion(r.Context(), userID)
	if err != nil {
		h.log.Warnw("failed to read prefs_version", "user_id", userID, "error", err)
		return
	}
	w.Header().Set("X-Prefs-Version", strconv.FormatInt(v, 10))
}

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

	// OptionalAuth: accept claims if present, otherwise pass empty userID to the service.
	// Empty userID skips Tier 1 (per-anime saved preference) and Tier 2 (user-global
	// history aggregation) — the resolver naturally falls through to Tier 3+ (community).
	// X-Anon-ID is captured into the metric label by the service layer, NOT by this handler.
	var userID string
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		userID = claims.UserID
	}

	resp, err := h.prefService.Resolve(r.Context(), userID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.writePrefsVersionHeader(w, r, userID)
	httputil.OK(w, resp)
}

// GetTier2DebugView returns the user's current Tier 2 weighted signals so
// the Advanced Settings UI can render "raw weights" and let the user
// understand why the resolver picked what it picked. Phase 7 B-05.
func (h *PreferenceHandler) GetTier2DebugView(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	view, err := h.prefService.GetTier2DebugView(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.writePrefsVersionHeader(w, r, claims.UserID)
	httputil.OK(w, view)
}

// ForceCombo writes a per-anime combo override as a Tier 1 preference.
// Phase 7 B-05 — the "force a specific combo" Advanced Settings action.
func (h *PreferenceHandler) ForceCombo(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.Error(w, errors.InvalidInput("anime_id is required"))
		return
	}

	var req domain.ForceComboRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if err := h.prefService.ForceCombo(r.Context(), claims.UserID, animeID, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("preference_force_combo",
		"user_id", claims.UserID,
		"anime_id", animeID,
		"player", req.Player,
		"language", req.Language,
		"watch_type", req.WatchType,
	)

	h.writePrefsVersionHeader(w, r, claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

// ResetLearnedPreferences deletes all per-anime preferences for the user.
// Watch history (Tier 2 source data) is preserved. Phase 7 B-05.
func (h *PreferenceHandler) ResetLearnedPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	newVersion, err := h.prefService.ResetLearnedPreferences(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("preference_reset_learned",
		"user_id", claims.UserID,
		"new_prefs_version", newVersion,
	)

	w.Header().Set("X-Prefs-Version", strconv.FormatInt(newVersion, 10))
	httputil.OK(w, map[string]int64{"prefs_version": newVersion})
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

	h.writePrefsVersionHeader(w, r, claims.UserID)
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

	h.writePrefsVersionHeader(w, r, claims.UserID)
	httputil.OK(w, map[string]interface{}{
		"top_combos": combos,
	})
}
