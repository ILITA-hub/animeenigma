package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// remoteProvider mirrors the JSON shape of catalog's
// GET /internal/scraper/providers response items.
type remoteProvider struct {
	Name             string `json:"name"`
	EngineKind       string `json:"engine_kind"`
	FailoverPriority int    `json:"failover_priority"`
	// Status is the DB's tri-state (enabled|degraded|disabled). Enabled is the
	// legacy bool kept ONLY to decode an older catalog mid-rolling-deploy; once
	// status is present it wins (see statusOf).
	Status           string `json:"status"`
	Enabled          *bool  `json:"enabled"`
	Group            string `json:"group"`
	Reason           string `json:"reason"`
	Description      string `json:"description"`
	ScraperOperated  bool   `json:"scraper_operated"`
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `json:"sub_delivery"`
	QualityCeiling   string `json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
	Engine           string `json:"engine"`
	BaseURL          string `json:"base_url"`
	Health           string `json:"health"`
}

// statusOf resolves the tri-state: prefer the new status field; fall back to the
// legacy enabled bool (older catalog during a rolling deploy); default enabled.
func (p remoteProvider) statusOf() ProviderStatus {
	switch ProviderStatus(p.Status) {
	case StatusEnabled, StatusDegraded, StatusDisabled:
		return ProviderStatus(p.Status)
	}
	if p.Enabled != nil && !*p.Enabled {
		return StatusDisabled
	}
	return StatusEnabled
}

// remoteResponse decodes catalog's standard {success,data:{...}} envelope.
type remoteResponse struct {
	Data struct {
		Providers []remoteProvider `json:"providers"`
	} `json:"data"`
}

// LoadProvidersRemote fetches provider config from catalog's internal endpoint
// and builds a ProvidersConfig (Source="remote"). AUTO-608: unlike LoadProviders,
// unknown provider names are ACCEPTED (fail-open) — a new DB row must never
// void the entire remote config and force a fallback. Activation is protected
// by the catalog's DB-contained engine_kind validation; main.go resolves that
// kind through the sole constructor registry. Group is derived from the
// intrinsic GroupOf(name), never trusting the remote value.
func LoadProvidersRemote(ctx context.Context, baseURL string, client *http.Client, timeout time.Duration) (ProvidersConfig, error) {
	if client == nil {
		client = &http.Client{}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/internal/scraper/providers"
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("fetch provider config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProvidersConfig{}, fmt.Errorf("provider config status %d", resp.StatusCode)
	}
	var rr remoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return ProvidersConfig{}, fmt.Errorf("decode provider config: %w", err)
	}

	metas := make(map[string]ProviderMeta, len(rr.Data.Providers))
	for _, p := range rr.Data.Providers {
		// The stream_providers roster holds EVERY stream source (ae + legacy
		// players + EN chain). The scraper operates ONLY scraper_operated rows;
		// first-party/legacy rows are skipped here so they never enter EN
		// failover.
		if !p.ScraperOperated {
			continue
		}
		if p.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("provider config: entry with empty name")
		}
		// AUTO-608 fail-open: names outside the compiled constructor set are
		// ACCEPTED (a new DB row must never make the scraper discard the whole
		// DB config and fall back to the offline default). Enabled rows carry a
		// DB-validated engine_kind resolved by main.go's constructor registry.
		if _, dup := metas[p.Name]; dup {
			return ProvidersConfig{}, fmt.Errorf("provider config: duplicate provider %q", p.Name)
		}
		subDelivery := p.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		metas[p.Name] = ProviderMeta{
			Name:             p.Name,
			EngineKind:       p.EngineKind,
			FailoverPriority: p.FailoverPriority,
			Status:           p.statusOf(),
			Reason:           p.Reason,
			Description:      p.Description,
			Group:            GroupOf(p.Name),
			SupportsSub:      p.SupportsSub,
			SupportsDub:      p.SupportsDub,
			SupportsRaw:      p.SupportsRaw,
			SubDelivery:      subDelivery,
			QualityCeiling:   p.QualityCeiling,
			PreferenceWeight: p.PreferenceWeight,
			Engine:           p.Engine,
			BaseURL:          p.BaseURL,
			Health:           p.Health,
		}
	}
	return newProvidersConfig(metas, "remote"), nil
}
