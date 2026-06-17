package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// AutocacheConfigStore is the slice of *repo.AutocacheConfigRepository
// the handler needs. Pulled out as an interface seam so tests can inject
// a stub without spinning up Postgres.
type AutocacheConfigStore interface {
	Get(ctx context.Context) (*domain.AutocacheConfig, error)
	Patch(ctx context.Context, fields map[string]any) (*domain.AutocacheConfig, error)
}

// AutocacheConfigHandler serves GET/PATCH /api/library/autocache/config
// — the live-editable autocache tunables + master `enabled` switch
// (POOL-04 + POOL-05). The gateway gates /api/library/* admin-only, so
// no server-side auth is enforced here.
type AutocacheConfigHandler struct {
	repo AutocacheConfigStore
	log  *logger.Logger
}

// NewAutocacheConfigHandler constructs the handler.
func NewAutocacheConfigHandler(repo AutocacheConfigStore, log *logger.Logger) *AutocacheConfigHandler {
	return &AutocacheConfigHandler{repo: repo, log: log}
}

// patchConfigRequest is the PATCH body shape. Every field is a pointer so
// an absent key stays nil and is NOT written (partial update). Only the
// non-nil fields reach the store, keyed by their DB column name.
type patchConfigRequest struct {
	Enabled               *bool  `json:"enabled"`
	BudgetBytes           *int64 `json:"budget_bytes"`
	AutoFreshDownloadDays *int   `json:"auto_fresh_download_days"`
	AutoFreshFetchDays    *int   `json:"auto_fresh_fetch_days"`
	AdminFreshDays        *int   `json:"admin_fresh_days"`
	ActiveWatcherDays     *int   `json:"active_watcher_days"`
	QualityCap            *int   `json:"quality_cap"`
	MinSeeders            *int   `json:"min_seeders"`
	SweepIntervalMin      *int   `json:"sweep_interval_min"`
}

// Get handles GET /api/library/autocache/config. Returns 200 with the
// {success,data} envelope wrapping every config field, or surfaces the
// repo error via httputil.Error.
func (h *AutocacheConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.repo.Get(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, cfg)
}

// Patch handles PATCH /api/library/autocache/config. Decodes the pointer
// body, range-validates each provided field, builds the column→value map
// from the non-nil pointers, and persists via the store. Returns 400 on
// a malformed body, an out-of-range value, or an empty (no writable keys)
// patch; otherwise 200 with the full updated config.
func (h *AutocacheConfigHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var body patchConfigRequest
	if err := httputil.Bind(r, &body); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	fields := map[string]any{}

	if body.Enabled != nil {
		fields["enabled"] = *body.Enabled
	}
	if body.BudgetBytes != nil {
		if *body.BudgetBytes <= 0 {
			httputil.BadRequest(w, "budget_bytes must be > 0")
			return
		}
		fields["budget_bytes"] = *body.BudgetBytes
	}
	if body.AutoFreshDownloadDays != nil {
		if *body.AutoFreshDownloadDays < 1 {
			httputil.BadRequest(w, "auto_fresh_download_days must be >= 1")
			return
		}
		fields["auto_fresh_download_days"] = *body.AutoFreshDownloadDays
	}
	if body.AutoFreshFetchDays != nil {
		if *body.AutoFreshFetchDays < 1 {
			httputil.BadRequest(w, "auto_fresh_fetch_days must be >= 1")
			return
		}
		fields["auto_fresh_fetch_days"] = *body.AutoFreshFetchDays
	}
	if body.AdminFreshDays != nil {
		if *body.AdminFreshDays < 1 {
			httputil.BadRequest(w, "admin_fresh_days must be >= 1")
			return
		}
		fields["admin_fresh_days"] = *body.AdminFreshDays
	}
	if body.ActiveWatcherDays != nil {
		if *body.ActiveWatcherDays < 1 {
			httputil.BadRequest(w, "active_watcher_days must be >= 1")
			return
		}
		fields["active_watcher_days"] = *body.ActiveWatcherDays
	}
	if body.QualityCap != nil {
		if *body.QualityCap <= 0 {
			httputil.BadRequest(w, "quality_cap must be > 0")
			return
		}
		fields["quality_cap"] = *body.QualityCap
	}
	if body.MinSeeders != nil {
		if *body.MinSeeders < 0 {
			httputil.BadRequest(w, "min_seeders must be >= 0")
			return
		}
		fields["min_seeders"] = *body.MinSeeders
	}
	if body.SweepIntervalMin != nil {
		if *body.SweepIntervalMin < 1 {
			httputil.BadRequest(w, "sweep_interval_min must be >= 1")
			return
		}
		fields["sweep_interval_min"] = *body.SweepIntervalMin
	}

	if len(fields) == 0 {
		httputil.BadRequest(w, "no config fields provided")
		return
	}

	cfg, err := h.repo.Patch(r.Context(), fields)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, cfg)
}
