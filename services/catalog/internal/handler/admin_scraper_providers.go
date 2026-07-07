package handler

import (
	"errors"
	"net/http"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// AdminScraperProvidersHandler exposes admin read/write endpoints over
// catalog's stream_providers table — a facade over ScraperProvider.Policy
// (spec 2026-07-07-rbac-roulette-p5-providers-facade-design.md §A1) so a
// future FE Providers tab can list providers and flip policy between
// auto/disabled at runtime. This is a pure DB read/write of the Policy
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

// SetPolicy handles PUT /api/admin/scraper-providers/{name}/policy. The only
// admin levers are auto/disabled — manual is a machine-set state driven by
// the probe state machine, so it (and any other value) is rejected with 400.
// Health/HealthSince are left untouched; only Policy + PolicySince change.
func (h *AdminScraperProvidersHandler) SetPolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req setPolicyRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	var policy domain.ProviderPolicy
	switch req.Policy {
	case string(domain.PolicyAuto):
		policy = domain.PolicyAuto
	case string(domain.PolicyDisabled):
		policy = domain.PolicyDisabled
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

	now := time.Now()
	if err := h.db.WithContext(r.Context()).Model(&domain.ScraperProvider{}).
		Where("name = ?", name).
		Updates(map[string]any{"policy": policy, "policy_since": now}).Error; err != nil {
		h.log.Errorw("failed to update scraper provider policy", "name", name, "error", err)
		httputil.Error(w, liberrors.Internal("failed to update scraper provider policy"))
		return
	}

	h.log.Infow("scraper provider policy updated", "name", name, "policy", policy)
	provider.Policy = policy
	provider.PolicySince = now
	httputil.OK(w, toAdminWire(provider))
}
