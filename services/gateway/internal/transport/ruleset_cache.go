package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// audience mirrors services/policy internal/domain.Audience (JSON shape).
// Duplicated (NOT imported) so the gateway stays decoupled from the policy
// service's internal packages; policy is the source of truth for the schema.
type audience struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
}

// rulesetSnapshot mirrors services/policy internal/domain.Ruleset.
type rulesetSnapshot struct {
	RouletteEnabled bool                `json:"rouletteEnabled"`
	Flags           map[string]audience `json:"flags"`
	FailSafe        map[string]string   `json:"failSafe"`
	Roulette        map[string]bool     `json:"roulette"`
}

// rulesetEnvelope is the httputil.OK wrapper policy-service returns.
type rulesetEnvelope struct {
	Success bool            `json:"success"`
	Data    rulesetSnapshot `json:"data"`
}

// rulesetFetchFunc fetches the current ruleset. Real impl GETs
// {policyURL}/internal/policy/ruleset; tests inject a fake.
type rulesetFetchFunc func(ctx context.Context) (rulesetSnapshot, error)

// rulesetCache holds the last-known-good ruleset in memory. Fail-static: a
// failed refresh keeps the previous snapshot. Until the first successful fetch
// loaded==false and FeatureGate falls back to each flag's failSafe.
type rulesetCache struct {
	mu     sync.RWMutex
	snap   rulesetSnapshot
	loaded bool
	fetch  rulesetFetchFunc
	log    *logger.Logger
}

func newRulesetCache(fetch rulesetFetchFunc, log *logger.Logger) *rulesetCache {
	return &rulesetCache{fetch: fetch, log: log}
}

func (c *rulesetCache) snapshot() (rulesetSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap, c.loaded
}

// refresh does one fetch; on error it logs and keeps the previous snapshot.
func (c *rulesetCache) refresh(ctx context.Context) {
	snap, err := c.fetch(ctx)
	if err != nil {
		c.log.Warnw("ruleset refresh failed; keeping last-known-good", "error", err)
		return
	}
	c.mu.Lock()
	c.snap = snap
	c.loaded = true
	c.mu.Unlock()
}

// Start does an immediate refresh, then refreshes every interval until ctx is done.
func (c *rulesetCache) Start(ctx context.Context, interval time.Duration) {
	c.refresh(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.refresh(ctx)
			}
		}
	}()
}

// httpRulesetFetch builds the production fetcher hitting policy-service's
// Docker-network-only ruleset feed (the gateway is on the same network).
func httpRulesetFetch(policyBaseURL string, client *http.Client) rulesetFetchFunc {
	return func(ctx context.Context) (rulesetSnapshot, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, policyBaseURL+"/internal/policy/ruleset", nil)
		if err != nil {
			return rulesetSnapshot{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return rulesetSnapshot{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return rulesetSnapshot{}, fmt.Errorf("policy ruleset: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return rulesetSnapshot{}, err
		}
		var env rulesetEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return rulesetSnapshot{}, err
		}
		return env.Data, nil
	}
}
