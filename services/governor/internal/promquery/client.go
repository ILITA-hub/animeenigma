// Package promquery is a minimal read-only client for the Prometheus
// instant-query HTTP API, tailored to the governor's needs: fetch every ae:*
// recording-rule series in ONE query per tick and fold them into a
// domain.Verdict, optionally fold in uplink-egress pressure, and measure the
// freshest sample's age so a slow-but-succeeding Prometheus is detectable.
package promquery

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

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// AllSignalsQuery matches every degradation recording rule
// (docker/prometheus/rules/degradation.yml) in a single instant vector.
const AllSignalsQuery = `{__name__=~"ae:.+"}`

// sampleAgeQuery yields the age (seconds) of the freshest pressure sample at
// query time. time() is the query-evaluation instant and timestamp() is the
// sample's own scrape/eval time, so the difference is the true staleness even
// when rule evaluation has lagged (a recording rule that pre-computed the age
// would itself go stale under that same lag, masking it).
//
// The scalar() wrapper is REQUIRED: timestamp() returns an instant VECTOR, so
// `time() - timestamp(...)` is a scalar-minus-vector that evaluates to a vector
// — which queryScalar cannot parse (it would leave the age unknown and the
// staleness guard silently disabled). scalar() collapses the single-element
// vector to a true Prometheus scalar.
const sampleAgeQuery = `scalar(time() - timestamp(ae:pressure_score:preview))`

const (
	breachElevatedName = "ae:pressure_breach:elevated"
	breachCriticalName = "ae:pressure_breach:critical"
	scoreName          = "ae:pressure_score:preview"
	egressSignalName   = "ae:host_egress:bytes_per_second"
	hostSignalPrefix   = "ae:host_"
)

// Client targets a Prometheus base URL such as
// "http://prometheus:9090/prometheus" (note the route-prefix).
type Client struct {
	baseURL    string
	httpClient *http.Client

	// Uplink-egress governance (opt-in). uplinkBytesPerSec == 0 disables it
	// entirely: an unknown uplink must never false-positive.
	uplinkBytesPerSec  float64
	egressElevatedFrac float64
	egressCriticalFrac float64
}

// NewClient builds a client. uplinkMbps <= 0 disables egress governance; the
// elevated/critical fractions are the share of that uplink at which egress
// pressure trips. The 5s timeout is generous for an instant query but well
// inside the 15s tick.
func NewClient(baseURL string, uplinkMbps, egressElevatedFrac, egressCriticalFrac float64) *Client {
	uplinkBps := 0.0
	if uplinkMbps > 0 {
		uplinkBps = uplinkMbps * 125000 // 1 Mbps = 1e6 bits/s ÷ 8 = 125000 bytes/s
	}
	return &Client{
		baseURL:            strings.TrimRight(baseURL, "/"),
		httpClient:         &http.Client{Timeout: 5 * time.Second},
		uplinkBytesPerSec:  uplinkBps,
		egressElevatedFrac: egressElevatedFrac,
		egressCriticalFrac: egressCriticalFrac,
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

type scalarResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"` // [ <ts number>, "<value string>" ]
	} `json:"data"`
}

// FetchVerdict runs the all-signals instant query and folds the result into a
// Verdict: instantaneous target level, active breach reasons (highest severity
// per signal, incl. governor-computed egress_uplink), raw host signal values,
// the raw pressure score, and the freshest-sample age.
func (c *Client) FetchVerdict(ctx context.Context) (domain.Verdict, error) {
	qr, err := c.queryVector(ctx)
	if err != nil {
		return domain.Verdict{}, err
	}

	v := domain.Verdict{Signals: map[string]float64{}, SampleAgeSeconds: -1}
	// Highest severity seen per breach signal (critical implies elevated — the
	// rule thresholds are strictly nested — so report critical only).
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
		case name == scoreName:
			v.Score = val
		case strings.HasPrefix(name, hostSignalPrefix):
			// Host rules are single-series; keep the max defensively if a
			// label split ever appears.
			if cur, exists := v.Signals[name]; !exists || val > cur {
				v.Signals[name] = val
			}
		}
	}

	// Fold in uplink-egress pressure (opt-in). Computed here rather than in a
	// recording rule so the uplink capacity stays governor config.
	c.foldEgress(&v, elevated, critical)

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

	// Best-effort staleness age: a failure leaves SampleAgeSeconds at -1
	// (unknown), which the governor treats as not-stale — never a false hold.
	if age, err := c.queryScalar(ctx, sampleAgeQuery); err == nil && age >= 0 {
		v.SampleAgeSeconds = age
	}
	return v, nil
}

// foldEgress adds egress_uplink to the breach maps and lifts the raw score when
// egress exceeds the configured fractions of uplink capacity.
func (c *Client) foldEgress(v *domain.Verdict, elevated, critical map[string]bool) {
	if c.uplinkBytesPerSec <= 0 {
		return
	}
	egressBps := v.Signals[egressSignalName]
	if egressBps <= 0 {
		return
	}
	frac := egressBps / c.uplinkBytesPerSec
	v.EgressFraction = frac
	switch {
	case frac >= c.egressCriticalFrac:
		critical[domain.EgressUplinkSignal] = true
	case frac >= c.egressElevatedFrac:
		elevated[domain.EgressUplinkSignal] = true
	}
	if s := egressScore(frac, c.egressElevatedFrac, c.egressCriticalFrac); s > v.Score {
		v.Score = s
	}
}

// egressScore mirrors the recording rules' per-signal normalization: 0 at
// half-elevated, 0.5 at elevated, 1.0 at critical.
func egressScore(frac, elev, crit float64) float64 {
	if elev <= 0 || crit <= elev {
		return 0
	}
	lo := 0.5 * clamp01((frac-elev/2)/(elev/2))
	hi := 0.5 * clamp01((frac-elev)/(crit-elev))
	return clamp01(lo + hi)
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// doQuery runs one instant query and decodes the response body into out (a
// *queryResponse or *scalarResponse). Owns the shared skeleton: endpoint build,
// GET, status-code check, decode. The per-shape "status" == success check and
// value extraction stay with the callers.
func (c *Client) doQuery(ctx context.Context, expr string, out any) error {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(expr))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("prometheus: decode: %w", err)
	}
	return nil
}

func (c *Client) queryVector(ctx context.Context) (*queryResponse, error) {
	var qr queryResponse
	if err := c.doQuery(ctx, AllSignalsQuery, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("prometheus: query status %q", qr.Status)
	}
	return &qr, nil
}

// queryScalar runs an instant query returning a Prometheus scalar and extracts
// its float value.
func (c *Client) queryScalar(ctx context.Context, expr string) (float64, error) {
	var sr scalarResponse
	if err := c.doQuery(ctx, expr, &sr); err != nil {
		return 0, err
	}
	if sr.Status != "success" {
		return 0, fmt.Errorf("prometheus: scalar status %q", sr.Status)
	}
	f, ok := sampleValue(sr.Data.Result)
	if !ok {
		return 0, fmt.Errorf("prometheus: bad scalar result")
	}
	return f, nil
}

// sampleValue extracts the float from the [ts, "value"] instant pair (shared by
// vector samples and the scalar result, which have the same shape).
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
