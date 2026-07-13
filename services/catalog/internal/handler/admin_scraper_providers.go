package handler

import (
	"errors"
	"net/http"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/providerpolicy"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// AdminScraperProvidersHandler exposes admin read/write endpoints over
// catalog's stream_providers table — a facade over ScraperProvider.Policy
// (spec 2026-07-07-rbac-roulette-p5-providers-facade-design.md §A1) so the
// FE Providers tab can list providers and flip policy between
// auto/manual/disabled at runtime. This is a pure DB read/write of the Policy
// column: it does NOT call or modify the self-heal engine
// (service/providerpolicy/engine.go), the probe pipeline, or capability
// derivation — Health stays probe-owned.
type AdminScraperProvidersHandler struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewAdminScraperProvidersHandler constructs the handler.
func NewAdminScraperProvidersHandler(db *gorm.DB, log *logger.Logger) *AdminScraperProvidersHandler {
	return &AdminScraperProvidersHandler{db: db, log: log}
}

// adminProviderWire extends the internal providerWire (internal_scraper_providers.go)
// with derived_state, the 5-state dashboard lifecycle label, so the FE
// Providers tab renders the status pill without re-implementing precedence.
type adminProviderWire struct {
	providerWire
	DerivedState string `json:"derived_state"`
}

func toAdminWire(p domain.ScraperProvider) adminProviderWire {
	return adminProviderWire{
		providerWire: toWire(p),
		DerivedState: p.DerivedState(),
	}
}

// List handles GET /api/admin/scraper-providers. Reuses the same query +
// wire mapping as the internal handler's List, extended with derived_state.
func (h *AdminScraperProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Order("name asc").Find(&rows).Error; err != nil {
		h.log.Errorw("failed to load scraper providers", "error", err)
		httputil.Error(w, liberrors.Internal("failed to load scraper providers"))
		return
	}
	wire := make([]adminProviderWire, len(rows))
	for i, p := range rows {
		wire[i] = toAdminWire(p)
	}
	httputil.OK(w, map[string]any{"providers": wire})
}

// setPolicyRequest is the PUT .../policy request body.
type setPolicyRequest struct {
	Policy string `json:"policy"`
}

// SetPolicy handles PUT /api/admin/scraper-providers/{name}/policy. As of
// 2026-07-13 the admin controls only a two-state PROBE STATUS (Auto / Disabled):
//   - "disabled" → policy=disabled, the hard-lock: dropped from playback + not
//     probed. The machine never lifts this.
//   - "auto"     → re-enable + hand back to the machine. Policy is set from the
//     provider's CURRENT health via the same rule the probe uses
//     (ReconcilePolicyFromHealth: health==down ⇒ manual, else auto), so
//     enabling a still-down provider reads back as manual (parked, hacker-
//     selectable) not a false "auto". The probe reconciles it on every tick.
// "manual" is no longer an admin input (it's machine-set) → rejected with 400.
// Health/HealthSince are left untouched; Policy + PolicySince change, and the
// derived Status is written alongside them (see below).
//
// The capability feed (GET /api/anime/{id}/capabilities — what the player
// Source panel consumes) filters disabled EN providers on the STORED `status`
// column, not `policy` (service/capability/service.go BuildENFamily: `WHERE
// status <> 'disabled'`). Every other writer of this table keeps `status` in
// lock-step with `policy` for the disabled case (BackfillPolicyHealth,
// AnimefeverDisable, AnimepaheBrowserRevival, … in service/scraperprovider/
// migrate.go) — that invariant is why an admin policy write must ALSO persist
// the derived status here (disable → status=disabled, manual park →
// status=degraded via WireStatus()): skipping it would leave the row
// split-brain (policy=disabled but status stale-enabled), so the feed keeps
// serving a provider that auto-failover — which derives from live policy via
// WireStatus() — has already stopped routing to.
func (h *AdminScraperProvidersHandler) SetPolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req setPolicyRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	// Only the two probe-status values are admin inputs. "manual" is machine-set
	// (health-driven) and rejected — the admin parks a provider by disabling it,
	// not by hand-picking manual.
	switch req.Policy {
	case string(domain.PolicyAuto), string(domain.PolicyDisabled):
		// ok
	default:
		httputil.BadRequest(w, `policy must be "auto" or "disabled"`)
		return
	}

	var provider domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Where("name = ?", name).First(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(w, "provider")
			return
		}
		h.log.Errorw("failed to load scraper provider", "name", name, "error", err)
		httputil.Error(w, liberrors.Internal("failed to load scraper provider"))
		return
	}

	// "disabled" locks; "auto" re-enables and hands the auto/manual axis back to
	// the machine — resolve it from CURRENT health via the same rule the probe
	// uses (ReconcilePolicyFromHealth), so a still-down provider re-enables as
	// manual (parked) rather than a false "auto".
	now := time.Now()
	policy := domain.ProviderPolicy(req.Policy)
	if policy != domain.PolicyDisabled {
		provider.Policy = domain.PolicyAuto // clear any prior disabled before reconcile
		providerpolicy.ReconcilePolicyFromHealth(&provider, now)
		policy = provider.Policy
	}

	// Derive the new stored status from the new policy + the provider's current
	// (probe-owned, untouched) health — exactly the mapping BackfillPolicyHealth/
	// the roster migrations use (disabled → disabled, manual → degraded) — so
	// the persisted column never lags behind policy for the cases the
	// capability feed's query relies on.
	provider.Policy = policy
	newStatus := provider.WireStatus()

	if err := h.db.WithContext(r.Context()).Model(&domain.ScraperProvider{}).
		Where("name = ?", name).
		Updates(map[string]any{"policy": policy, "policy_since": now, "status": newStatus}).Error; err != nil {
		h.log.Errorw("failed to update scraper provider policy", "name", name, "error", err)
		httputil.Error(w, liberrors.Internal("failed to update scraper provider policy"))
		return
	}

	h.log.Infow("scraper provider policy updated", "name", name, "policy", policy, "status", newStatus)
	provider.PolicySince = now
	provider.Status = newStatus
	httputil.OK(w, toAdminWire(provider))
}
