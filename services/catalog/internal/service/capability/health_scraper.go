package capability

import (
	"context"
	"encoding/json"
	"fmt"
)

// scraperHealthClient is the minimal interface for fetching the raw
// /scraper/health response body from the scraper microservice.
// *service.CatalogService satisfies this via its GetScraperHealth method.
type scraperHealthClient interface {
	GetScraperHealth(ctx context.Context) (int, []byte, error)
}

// ScraperHealth adapts the scraper /scraper/health body to HealthSource.
type ScraperHealth struct{ client scraperHealthClient }

// NewScraperHealth wraps a scraperHealthClient as a HealthSource.
func NewScraperHealth(c scraperHealthClient) ScraperHealth { return ScraperHealth{client: c} }

// ProviderHealth calls the scraper health endpoint and decodes the response.
// The scraper response shape is:
//
//	{
//	  "success": true,
//	  "data": {
//	    "providers": { "<name>": { "up": true, "provider": "...", "stages": {...}, ... } },
//	    "playable":  { "<name>": true }
//	  }
//	}
//
// The `up` field is a top-level bool per provider entry (Task 3 enrichment).
// Providers absent from `playable` get a nil Playable pointer (unknown).
func (s ScraperHealth) ProviderHealth(ctx context.Context) (map[string]HealthInfo, error) {
	status, body, err := s.client.GetScraperHealth(ctx)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("scraper health status %d", status)
	}
	var env struct {
		Data struct {
			Providers map[string]struct {
				Up bool `json:"up"`
			} `json:"providers"`
			Playable map[string]bool `json:"playable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode scraper health: %w", err)
	}
	out := make(map[string]HealthInfo, len(env.Data.Providers))
	for name, p := range env.Data.Providers {
		hi := HealthInfo{Up: p.Up}
		if pb, ok := env.Data.Playable[name]; ok {
			v := pb
			hi.Playable = &v
		}
		out[name] = hi
	}
	return out, nil
}
