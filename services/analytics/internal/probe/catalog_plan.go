package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PlanClient abstracts the catalog probe-plan API so the engine can be tested
// without a real HTTP server.
type PlanClient interface {
	FetchPlan(ctx context.Context) ([]PlanEntry, error)
	PostVerdict(ctx context.Context, provider string, pass bool, reason string, metrics *TickMetrics) error
}

type httpPlanClient struct {
	catalogURL string
	hc         *http.Client
}

// NewHTTPPlanClient returns a PlanClient backed by catalog's internal API.
func NewHTTPPlanClient(catalogURL string, hc *http.Client) PlanClient {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &httpPlanClient{catalogURL: catalogURL, hc: hc}
}

func (c *httpPlanClient) FetchPlan(ctx context.Context) ([]PlanEntry, error) {
	return FetchPlan(ctx, c.catalogURL, c.hc)
}

func (c *httpPlanClient) PostVerdict(ctx context.Context, p string, pass bool, reason string, metrics *TickMetrics) error {
	return PostVerdict(ctx, c.catalogURL, c.hc, p, pass, reason, metrics)
}

// PlanEntry is one provider the catalog says is due to probe this tick.
type PlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
	Engine     string `json:"engine"`
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

// PostVerdict reports a provider's probe pass/fail (+ optional last-tick metrics)
// to catalog's state machine. Calls POST /internal/providers/probe-result with
// body {"provider":...,"pass":...,"reason":...} as expected by catalog's
// InternalProviderPolicyHandler.ProbeResult. metrics may be nil (omitted from
// the body) for backward compatibility.
func PostVerdict(ctx context.Context, catalogURL string, c *http.Client, provider string, pass bool, reason string, metrics *TickMetrics) error {
	body := map[string]any{"provider": provider, "pass": pass, "reason": reason}
	if metrics != nil {
		body["metrics"] = metrics
	}
	payload, _ := json.Marshal(body)
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
