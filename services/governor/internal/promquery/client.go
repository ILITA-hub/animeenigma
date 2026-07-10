// Package promquery is a minimal read-only client for the Prometheus
// instant-query HTTP API, tailored to the governor's single need: fetch every
// ae:* recording-rule series in ONE query per tick and fold them into a
// domain.Verdict. Mirrors the catalog spotlight prometheus client, but keeps
// labels (the breach rules carry their identity in the `signal` label).
package promquery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// AllSignalsQuery matches every degradation recording rule
// (docker/prometheus/rules/degradation.yml) in a single instant vector.
const AllSignalsQuery = `{__name__=~"ae:.+"}`

const (
	breachElevatedName = "ae:pressure_breach:elevated"
	breachCriticalName = "ae:pressure_breach:critical"
	hostSignalPrefix   = "ae:host_"
)

// Client targets a Prometheus base URL such as
// "http://prometheus:9090/prometheus" (note the route-prefix).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a client. The 5s timeout is generous for an instant query
// but well inside the 15s tick.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"` // [ <ts>, "<value>" ]
		} `json:"result"`
	} `json:"data"`
}

// FetchVerdict runs the all-signals instant query and folds the result into a
// Verdict: instantaneous target level, active breach reasons (highest severity
// per signal), and the raw host signal values.
func (c *Client) FetchVerdict(ctx context.Context) (domain.Verdict, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(AllSignalsQuery))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Verdict{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Verdict{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.Verdict{}, fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}
	var qr queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return domain.Verdict{}, fmt.Errorf("prometheus: decode: %w", err)
	}
	if qr.Status != "success" {
		return domain.Verdict{}, fmt.Errorf("prometheus: query status %q", qr.Status)
	}

	v := domain.Verdict{Signals: map[string]float64{}}
	// Highest severity seen per breach signal (critical implies elevated —
	// the rule thresholds are strictly nested — so report critical only).
	critical := map[string]bool{}
	elevated := map[string]bool{}

	for _, s := range qr.Data.Result {
		name := s.Metric["__name__"]
		val, ok := sampleValue(s.Value)
		if !ok {
			continue
		}
		switch {
		case name == breachCriticalName:
			if val >= 1 {
				critical[s.Metric["signal"]] = true
			}
		case name == breachElevatedName:
			if val >= 1 {
				elevated[s.Metric["signal"]] = true
			}
		case strings.HasPrefix(name, hostSignalPrefix):
			// Host rules are single-series; keep the max defensively if a
			// label split ever appears.
			if cur, exists := v.Signals[name]; !exists || val > cur {
				v.Signals[name] = val
			}
		}
	}

	// Fixed signal order keeps reasons (and the transition log) deterministic.
	for _, sig := range domain.BreachSignals {
		switch {
		case critical[sig]:
			v.Reasons = append(v.Reasons, domain.Reason{Signal: sig, Severity: domain.SeverityCritical})
		case elevated[sig]:
			v.Reasons = append(v.Reasons, domain.Reason{Signal: sig, Severity: domain.SeverityElevated})
		}
	}
	switch {
	case len(critical) > 0:
		v.Target = domain.LevelCritical
	case len(elevated) > 0:
		v.Target = domain.LevelElevated
	default:
		v.Target = domain.LevelNormal
	}
	return v, nil
}

// sampleValue extracts the float from the [ts, "value"] instant-vector pair.
func sampleValue(pair []json.RawMessage) (float64, bool) {
	if len(pair) != 2 {
		return 0, false
	}
	var raw string
	if err := json.Unmarshal(pair[1], &raw); err != nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
