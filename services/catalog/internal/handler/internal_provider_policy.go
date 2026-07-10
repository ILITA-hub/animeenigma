package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/providerpolicy"
	"gorm.io/gorm"
)

// InternalProviderPolicyHandler applies probe verdicts to the provider state
// machine via POST /internal/providers/probe-result. Reachable only from
// within the Docker network (the gateway does not proxy /internal/*).
type InternalProviderPolicyHandler struct {
	db  *gorm.DB
	cfg config.ProviderPolicyConfig
	log *logger.Logger
}

// NewInternalProviderPolicyHandler constructs the handler.
func NewInternalProviderPolicyHandler(db *gorm.DB, cfg config.ProviderPolicyConfig, log *logger.Logger) *InternalProviderPolicyHandler {
	return &InternalProviderPolicyHandler{db: db, cfg: cfg, log: log}
}

type probeResultReq struct {
	Provider string          `json:"provider"`
	Pass     bool            `json:"pass"`
	Reason   string          `json:"reason"`
	Metrics  json.RawMessage `json:"metrics"`
}

// ProbeResult handles POST /internal/providers/probe-result.
//
// Body: {"provider":"gogoanime","pass":false,"reason":"status_403"}
// Response on success: {"success":true,"data":{"provider","policy","health"}}
// Response on skip (disabled or !scraper_operated): {"success":true,"data":{..., "skipped":true}}
//
// Repeated calls with the same verdict converge (fail walks up→degraded→down,
// then down is absorbing; pass climbs back to up). Policy is admin-only and
// never mutated here — ApplyVerdict advances health alone.
func (h *InternalProviderPolicyHandler) ProbeResult(w http.ResponseWriter, r *http.Request) {
	var req probeResultReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider == "" {
		http.Error(w, `{"success":false,"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	var p domain.ScraperProvider
	if err := h.db.First(&p, "name = ?", req.Provider).Error; err != nil {
		http.Error(w, `{"success":false,"error":"unknown provider"}`, http.StatusNotFound)
		return
	}

	// disabled is the hard lock; non-scraper rows are not under policy management.
	// Their provider_state gauge is seeded once at boot (EmitProviderStates) and,
	// like provider_info/provider_enabled, only changes across a restart — so there
	// is nothing to re-emit here.
	if p.Policy == domain.PolicyDisabled || !p.ScraperOperated {
		httputil.OK(w, map[string]any{
			"provider": p.Name,
			"policy":   p.Policy,
			"health":   p.Health,
			"skipped":  true,
		})
		return
	}

	now := time.Now().UTC()
	if req.Reason != "" {
		p.Reason = req.Reason
	}
	providerpolicy.ApplyVerdict(&p, req.Pass, now, h.cfg.PromoteAfter)

	// policy/policy_since are NOT written: ApplyVerdict never mutates policy
	// (admin-only), and omitting them also closes the write-back window where
	// an admin's auto→manual park racing this handler would be clobbered by
	// the stale value read above.
	updates := map[string]any{
		"health":         p.Health,
		"health_since":   p.HealthSince,
		"last_probed_at": p.LastProbedAt,
		"reason":         p.Reason,
	}
	if m := bytes.TrimSpace(req.Metrics); len(m) > 0 && !bytes.Equal(m, []byte("null")) && !bytes.Equal(m, []byte("{}")) {
		updates["last_tick_metrics"] = string(m)
	}

	// disabled is a hard lock: guard the write with "policy <> disabled" so a
	// row that an admin disabled in the window between our First() read above
	// and this Updates() (TOCTOU) is NOT clobbered back to a live policy —
	// without this, a probe verdict racing an admin's SetPolicy(disabled)
	// could silently re-enable a provider the admin just turned off.
	result := h.db.Model(&domain.ScraperProvider{}).
		Where("name = ? AND policy <> ?", p.Name, domain.PolicyDisabled).
		Updates(updates)
	if result.Error != nil {
		h.log.Errorw("probe-result persist failed", "provider", p.Name, "error", result.Error)
		http.Error(w, `{"success":false,"error":"persist failed"}`, http.StatusInternalServerError)
		return
	}
	if result.RowsAffected == 0 {
		// Lost the race: the row was disabled after our read. Don't emit a
		// gauge/response built from the stale (pre-disable) in-memory verdict.
		httputil.OK(w, map[string]any{
			"provider": p.Name,
			"policy":   domain.PolicyDisabled,
			"health":   p.Health,
			"skipped":  true,
		})
		return
	}

	// Reflect the post-transition derived state into the provider_state gauge so
	// the "Provider State History" timeline records this transition live (the
	// gauge holds between probes; Prometheus scraping fills the continuous band).
	metrics.ProviderState.WithLabelValues(p.Name, p.Group).Set(p.StateCode())

	httputil.OK(w, map[string]any{
		"provider": p.Name,
		"policy":   p.Policy,
		"health":   p.Health,
	})
}

type probePlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
	Engine     string `json:"engine"`
}

// ProbePlan handles GET /internal/providers/probe-plan.
//
// Returns the cadence-gated due-set of providers that should be probed now,
// together with per-provider sample size and fail-fast flag. Disabled providers
// are always excluded. Non-scraper-operated providers use a fixed 24h cadence
// with sample_size=1 and fail_fast=true. Scraper-operated providers use the
// state-machine cadence (ProbeCadence/ProbeSample) from the domain helpers.
//
// Response: {"success":true,"data":{"plan":[{"provider":"...","sample_size":5,"fail_fast":false}]}}
func (h *InternalProviderPolicyHandler) ProbePlan(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.Order("name asc").Find(&rows).Error; err != nil {
		http.Error(w, `{"success":false,"error":"db"}`, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	plan := make([]probePlanEntry, 0, len(rows))
	for _, p := range rows {
		if p.Policy == domain.PolicyDisabled {
			continue
		}
		var cadence time.Duration
		var size int
		var ff bool
		if p.ScraperOperated {
			cadence = p.ProbeCadence(h.cfg.Cadence)
			size, ff = p.ProbeSample(h.cfg.Cadence)
		} else {
			cadence = h.cfg.Cadence.Manual // non-scraper: fixed daily
			size, ff = 1, true
		}
		if cadence <= 0 || now.Sub(p.LastProbedAt) < cadence {
			continue
		}
		plan = append(plan, probePlanEntry{Provider: p.Name, SampleSize: size, FailFast: ff, Engine: p.Engine})
	}
	httputil.OK(w, map[string]any{"plan": plan})
}
