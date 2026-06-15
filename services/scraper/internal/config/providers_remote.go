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
	Enabled          bool   `json:"enabled"`
	Group            string `json:"group"`
	Reason           string `json:"reason"`
	Description      string `json:"description"`
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `json:"sub_delivery"`
	QualityCeiling   string `json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
}

type remoteResponse struct {
	Providers []remoteProvider `json:"providers"`
}

// LoadProvidersRemote fetches provider config from catalog's internal endpoint
// and builds a ProvidersConfig (Source="remote"). Unknown provider names are
// rejected (same fail-fast invariant as LoadProviders) so a bad DB row falls
// back to YAML at the call site rather than silently mis-registering. Group is
// derived from the intrinsic GroupOf(name), never trusting the remote value.
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

	known := make(map[string]bool, len(KnownProviders))
	for _, n := range KnownProviders {
		known[n] = true
	}
	metas := make(map[string]ProviderMeta, len(rr.Providers))
	for _, p := range rr.Providers {
		if p.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("provider config: entry with empty name")
		}
		if !known[p.Name] {
			return ProvidersConfig{}, fmt.Errorf("provider config: unknown provider %q", p.Name)
		}
		if _, dup := metas[p.Name]; dup {
			return ProvidersConfig{}, fmt.Errorf("provider config: duplicate provider %q", p.Name)
		}
		subDelivery := p.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		metas[p.Name] = ProviderMeta{
			Name:             p.Name,
			Enabled:          p.Enabled,
			Reason:           p.Reason,
			Description:      p.Description,
			Group:            GroupOf(p.Name),
			SupportsSub:      p.SupportsSub,
			SupportsDub:      p.SupportsDub,
			SupportsRaw:      p.SupportsRaw,
			SubDelivery:      subDelivery,
			QualityCeiling:   p.QualityCeiling,
			PreferenceWeight: p.PreferenceWeight,
		}
	}
	return ProvidersConfig{metas: metas, Source: "remote"}, nil
}
