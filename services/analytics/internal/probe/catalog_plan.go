package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// PlanEntry is one provider the catalog says is due to probe this tick.
type PlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
}

// FetchPlan asks catalog which providers are due now and how to probe each.
// Calls GET /internal/providers/probe-plan and decodes the
// {"success":true,"data":{"plan":[...]}} envelope emitted by catalog's
// InternalProviderPolicyHandler.ProbePlan.
func FetchPlan(ctx context.Context, catalogURL string, c *http.Client) ([]PlanEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL+"/internal/providers/probe-plan", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("probe-plan status %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Plan []PlanEntry `json:"plan"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Data.Plan, nil
}

// PostVerdict reports a provider's probe pass/fail to catalog's state machine.
// Calls POST /internal/providers/probe-result with body
// {"provider":...,"pass":...,"reason":...} as expected by catalog's
// InternalProviderPolicyHandler.ProbeResult.
func PostVerdict(ctx context.Context, catalogURL string, c *http.Client, provider string, pass bool, reason string) error {
	payload, _ := json.Marshal(map[string]any{"provider": provider, "pass": pass, "reason": reason})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, catalogURL+"/internal/providers/probe-result", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe-result status %d", resp.StatusCode)
	}
	return nil
}
