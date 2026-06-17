package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// PATCH validation ceilings. These tunables drive the future downloader/evictor
// budget ledger and are admin-editable live (no redeploy), so a floor-only check
// lets an admin set absurd values (a 9.2 EB budget, a ~3800-year sweep interval)
// with no warning. Each numeric field gets a documented upper bound; values
// outside [floor, ceiling] are rejected with 400.
const (
	// maxBudgetBytes caps the pool budget at 100 TiB — three orders of magnitude
	// above the 100 GiB default (§3.5), generous for a self-hosted deployment
	// while still rejecting an obvious fat-finger.
	maxBudgetBytes int64 = 100 * 1024 * 1024 * 1024 * 1024

	// maxFreshDays bounds every *_fresh_*_days / active_watcher_days window at
	// ten years — far beyond any sane freshness/recency window.
	maxFreshDays = 3650

	// maxMinSeeders bounds min_seeders; no real swarm needs a five-figure floor.
	maxMinSeeders = 10000

	// maxSweepIntervalMin caps the planner cadence at one week. Past that the
	// evictor effectively never runs (the spec default is 20m, §3.5).
	maxSweepIntervalMin = 7 * 24 * 60
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
		if *body.BudgetBytes <= 0 || *body.BudgetBytes > maxBudgetBytes {
			httputil.BadRequest(w, "budget_bytes must be in 1..107374182400000 (100 TiB)")
			return
		}
		fields["budget_bytes"] = *body.BudgetBytes
	}
	if body.AutoFreshDownloadDays != nil {
		if *body.AutoFreshDownloadDays < 1 || *body.AutoFreshDownloadDays > maxFreshDays {
			httputil.BadRequest(w, "auto_fresh_download_days must be in 1..3650")
			return
		}
		fields["auto_fresh_download_days"] = *body.AutoFreshDownloadDays
	}
	if body.AutoFreshFetchDays != nil {
		if *body.AutoFreshFetchDays < 1 || *body.AutoFreshFetchDays > maxFreshDays {
			httputil.BadRequest(w, "auto_fresh_fetch_days must be in 1..3650")
			return
		}
		fields["auto_fresh_fetch_days"] = *body.AutoFreshFetchDays
	}
	if body.AdminFreshDays != nil {
		if *body.AdminFreshDays < 1 || *body.AdminFreshDays > maxFreshDays {
			httputil.BadRequest(w, "admin_fresh_days must be in 1..3650")
			return
		}
		fields["admin_fresh_days"] = *body.AdminFreshDays
	}
	if body.ActiveWatcherDays != nil {
		if *body.ActiveWatcherDays < 1 || *body.ActiveWatcherDays > maxFreshDays {
			httputil.BadRequest(w, "active_watcher_days must be in 1..3650")
			return
		}
		fields["active_watcher_days"] = *body.ActiveWatcherDays
	}
	if body.QualityCap != nil {
		// quality_cap is a discrete vertical-resolution ladder, not a free int:
		// the Phase-8 downloader compares it against real stream heights, so a
		// value like 137 silently filters everything-or-nothing. v1 cap is 1080
		// (§3.5 / D4); 2160 is reserved for the v2 TODO but accepted as a valid
		// enum member so the table need not change when that ships.
		switch *body.QualityCap {
		case 480, 720, 1080, 2160:
			fields["quality_cap"] = *body.QualityCap
		default:
			httputil.BadRequest(w, "quality_cap must be one of 480, 720, 1080, 2160")
			return
		}
	}
	if body.MinSeeders != nil {
		if *body.MinSeeders < 0 || *body.MinSeeders > maxMinSeeders {
			httputil.BadRequest(w, "min_seeders must be in 0..10000")
			return
		}
		fields["min_seeders"] = *body.MinSeeders
	}
	if body.SweepIntervalMin != nil {
		if *body.SweepIntervalMin < 1 || *body.SweepIntervalMin > maxSweepIntervalMin {
			httputil.BadRequest(w, "sweep_interval_min must be in 1..10080 (1 week)")
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
