package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// playabilityFetchTimeout bounds the synchronous read on the report path.
const playabilityFetchTimeout = 3 * time.Second

// PlayabilitySource supplies per-provider decayed playability scores for an
// anime. Best-effort: implementations MUST return an empty map (never error)
// on any failure so report assembly is never blocked.
type PlayabilitySource interface {
	Scores(ctx context.Context, animeID string) map[string]providerScore
}

// playabilityClient reads /internal/playability from analytics. Mirrors the
// libs/tracing producer posture (short timeout, swallow all errors).
type playabilityClient struct {
	url     string // "<ANALYTICS_INTERNAL_URL>/internal/playability"
	client  *http.Client
	enabled bool
}

// NewPlayabilityClient builds the read client. enabled=false (kill switch) makes
// Scores a no-op returning nil.
func NewPlayabilityClient(analyticsURL string, enabled bool) *playabilityClient {
	return &playabilityClient{
		url:     analyticsURL + "/internal/playability",
		client:  &http.Client{Timeout: playabilityFetchTimeout},
		enabled: enabled,
	}
}

type playabilityWire struct {
	Providers map[string]providerScore `json:"providers"`
}

// Scores fetches and alias-merges per-provider scores. Any failure → nil map
// (health-only index, no promotions) + a fetch-failure metric.
func (c *playabilityClient) Scores(ctx context.Context, animeID string) map[string]providerScore {
	if c == nil || !c.enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, playabilityFetchTimeout)
	defer cancel()

	u := c.url + "?anime_id=" + url.QueryEscape(animeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	resp, err := c.client.Do(req)
	if err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	var wire playabilityWire
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	return applyProviderAliases(wire.Providers)
}
