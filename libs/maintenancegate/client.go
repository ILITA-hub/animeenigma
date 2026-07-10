// Package maintenancegate is a fail-open client for policy-service's
// /internal/maintenance/routines gate + status endpoints, shared by the
// scheduler and the host-native maintenance daemon.
package maintenancegate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: timeout}}
}

type gateEnvelope struct {
	Data struct {
		Enabled  bool           `json:"enabled"`
		Settings map[string]any `json:"settings"`
	} `json:"data"`
}

func (c *Client) fetch(ctx context.Context, id string) (*gateEnvelope, bool) {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s", c.base, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var env gateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, false
	}
	return &env, true
}

// Enabled reports whether the routine may run. FAIL-OPEN: any error
// (unreachable, non-200, parse-fail) returns true so a policy outage never
// silently pauses a routine.
func (c *Client) Enabled(ctx context.Context, id string) bool {
	env, ok := c.fetch(ctx, id)
	if !ok {
		return true
	}
	return env.Data.Enabled
}

// MaxRisk returns the auto_apply_max_risk knob ("none"/"low"/"medium"), or ""
// on any error/miss (fail-open = no cap).
func (c *Client) MaxRisk(ctx context.Context, id string) string {
	env, ok := c.fetch(ctx, id)
	if !ok {
		return ""
	}
	if v, ok := env.Data.Settings["auto_apply_max_risk"].(string); ok {
		return v
	}
	return ""
}

// PostStatus stamps last-run status. Fire-and-forget: errors are ignored so a
// status write never affects the routine.
func (c *Client) PostStatus(ctx context.Context, id string, ok bool, summary string) {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s/status", c.base, id)
	body, err := json.Marshal(map[string]any{"ok": ok, "summary": summary})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
