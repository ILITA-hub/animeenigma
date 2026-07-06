package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// InternalScraperProvidersHandler serves the scraper provider config + capability
// traits to the scraper service over the Docker network.
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as /internal/cache/invalidate/raw/ and
// /internal/anime/{shikimoriId}/episodes (see internal_cache.go for the precedent).
// The gateway does NOT proxy /internal/*, so the route is reachable only from
// within the Docker network (spec 2026-06-15-scraper-capability-api).
type InternalScraperProvidersHandler struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewInternalScraperProvidersHandler constructs the handler.
func NewInternalScraperProvidersHandler(db *gorm.DB, log *logger.Logger) *InternalScraperProvidersHandler {
	return &InternalScraperProvidersHandler{db: db, log: log}
}

// providerWire is the wire representation of a ScraperProvider.
// status is DERIVED via WireStatus() (policy+health → tri-state), not read from
// the stored column. All other fields mirror the domain model 1:1 so the scraper
// consumer's field expectations are unchanged.
type providerWire struct {
	Name             string    `json:"name"`
	Status           string    `json:"status"` // derived via WireStatus(), NOT the stored column
	Policy           string    `json:"policy"`
	Health           string    `json:"health"`
	HealthSince      time.Time `json:"health_since"`
	PolicySince      time.Time `json:"policy_since"`
	LastProbedAt     time.Time `json:"last_probed_at"`
	Group            string    `json:"group"`
	Reason           string    `json:"reason"`
	Description      string    `json:"description"`
	ScraperOperated  bool      `json:"scraper_operated"`
	SupportsSub      bool      `json:"supports_sub"`
	SupportsDub      bool      `json:"supports_dub"`
	SupportsRaw      bool      `json:"supports_raw"`
	SubDelivery      string    `json:"sub_delivery"`
	QualityCeiling   string    `json:"quality_ceiling"`
	PreferenceWeight int       `json:"preference_weight"`
	Engine           string    `json:"engine"`
	BaseURL          string    `json:"base_url"`
	LastTickMetrics  string    `json:"last_tick_metrics"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// toWire maps a domain.ScraperProvider to providerWire, deriving status from
// WireStatus() so the scraper receives the computed tri-state (enabled/degraded/disabled)
// rather than the persisted column (which may lag behind Policy/Health).
func toWire(p domain.ScraperProvider) providerWire {
	return providerWire{
		Name:             p.Name,
		Status:           string(p.WireStatus()),
		Policy:           string(p.Policy),
		Health:           string(p.Health),
		HealthSince:      p.HealthSince,
		PolicySince:      p.PolicySince,
		LastProbedAt:     p.LastProbedAt,
		Group:            p.Group,
		Reason:           p.Reason,
		Description:      p.Description,
		ScraperOperated:  p.ScraperOperated,
		SupportsSub:      p.SupportsSub,
		SupportsDub:      p.SupportsDub,
		SupportsRaw:      p.SupportsRaw,
		SubDelivery:      p.SubDelivery,
		QualityCeiling:   p.QualityCeiling,
		PreferenceWeight: p.PreferenceWeight,
		Engine:           p.Engine,
		BaseURL:          p.BaseURL,
		LastTickMetrics:  p.LastTickMetrics,
		UpdatedAt:        p.UpdatedAt,
	}
}

// List handles GET /internal/scraper/providers.
// Returns all domain.ScraperProvider rows ordered by name as
// {"providers":[...]} inside the standard {success,data:{...}} envelope.
// The wire status field is DERIVED via WireStatus() (not the stored column).
func (h *InternalScraperProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Order("name asc").Find(&rows).Error; err != nil {
		h.log.Errorw("failed to load scraper providers", "error", err)
		httputil.Error(w, errors.Internal("failed to load scraper providers"))
		return
	}
	wire := make([]providerWire, len(rows))
	for i, p := range rows {
		wire[i] = toWire(p)
	}
	httputil.OK(w, map[string]any{"providers": wire})
}
