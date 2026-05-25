// Package prometheus is a minimal read-only client for the Prometheus
// instant-query HTTP API, used by the catalog spotlight platform_stats
// card. It intentionally supports only the "vector" result shape we emit
// (sum/count/avg aggregations always return a vector).
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client targets a Prometheus base URL such as
// "http://prometheus:9090/prometheus" (note the route-prefix). The instant
// query endpoint is "{base}/api/v1/query".
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a client with an 800ms timeout to respect the spotlight
// aggregator's per-card deadline.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 800 * time.Millisecond},
	}
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value []json.RawMessage `json:"value"` // [ <ts>, "<value>" ]
		} `json:"result"`
	} `json:"data"`
}

// Query runs an instant PromQL query and returns the first sample value.
// Returns an error on transport failure, non-200, decode failure, or empty
// result.
func (c *Client) Query(ctx context.Context, promql string) (float64, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promql))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}
	var qr queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return 0, fmt.Errorf("prometheus: decode query response: %w", err)
	}
	if qr.Status != "success" || len(qr.Data.Result) == 0 {
		return 0, fmt.Errorf("prometheus: no successful result for %q (status %q)", promql, qr.Status)
	}
	v := qr.Data.Result[0].Value
	if len(v) != 2 {
		return 0, fmt.Errorf("prometheus: malformed value")
	}
	var s string
	if err := json.Unmarshal(v[1], &s); err != nil {
		return 0, fmt.Errorf("prometheus: unmarshal sample value: %w", err)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("prometheus: parse sample value %q: %w", s, err)
	}
	return f, nil
}

// Health reports whether all scrape targets are up and the 7-day average
// uptime percentage (0..100). If the first query fails, all return values
// are zero-value and err is non-nil. If the second (uptime) query fails,
// allUp is still populated but uptimePct is 0 and err is non-nil. Callers
// treat any non-nil error as "Prometheus unreachable".
func (c *Client) Health(ctx context.Context) (allUp bool, uptimePct float64, err error) {
	down, err := c.Query(ctx, "count(up == 0) or vector(0)")
	if err != nil {
		return false, 0, err
	}
	allUp = down == 0
	avg, err := c.Query(ctx, "avg(avg_over_time(up[7d]))")
	if err != nil {
		return allUp, 0, err
	}
	uptimePct = math.Max(0, math.Min(100, avg*100))
	return allUp, uptimePct, nil
}
