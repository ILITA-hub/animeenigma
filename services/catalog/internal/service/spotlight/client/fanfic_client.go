// Daily Fanfic Spotlight — Task 11.
//
// FanficClient is a thin HTTP wrapper around the catalog → fanfic fan-out
// the spotlight aggregator's daily_fanfic resolver needs.
//
// FetchDaily → GET http://fanfic:8097/internal/fanfic/daily
//
// The endpoint is docker-network-only (no JWT — see services/fanfic's
// router.go), and returns httputil.OK's {success, data} envelope wrapping
// service.DailyDTO (mirrored locally as spotlight.DailyFanficData — see
// types.go's doc comment). A 404 (httputil.NotFound) means "no daily
// fanfic eligible today" — NOT an error — and this client treats it as
// (nil, nil) so the resolver can distinguish "ineligible" from "failed".
//
// Pattern mirror of services/catalog/internal/service/spotlight/client/player_client.go
// (Phase 3's HTTP client to the player container): same 700ms default
// timeout, same logger discipline, same LimitReader-on-error-body.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// defaultFanficBaseURL is the docker-network DNS name + port the catalog
// service uses to reach the fanfic container.
const defaultFanficBaseURL = "http://fanfic:8097"

// defaultFanficTimeout is tighter than the spotlight aggregator's 800ms
// per-card budget (HSB-BE-03), so the HTTP transport surfaces a fast
// failure BEFORE the resolver's parent context deadline trips.
const defaultFanficTimeout = 700 * time.Millisecond

// dailyFanficEnvelope is the fanfic service's wire format for
// GET /internal/fanfic/daily. The handler writes it via libs/httputil.OK,
// which produces {success: bool, data: {...}}. We only care about data.
type dailyFanficEnvelope struct {
	Data spotlight.DailyFanficData `json:"data"`
}

// FanficClient fans out HTTP calls to the fanfic service.
type FanficClient struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger
}

// NewFanficClient constructs a FanficClient. Empty baseURL → "http://fanfic:8097".
// Nil hc → an http.Client with the 700ms default Timeout. log MUST be non-nil
// (production wires the same *logger.Logger the rest of catalog uses).
func NewFanficClient(baseURL string, hc *http.Client, log *logger.Logger) *FanficClient {
	if baseURL == "" {
		baseURL = defaultFanficBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultFanficTimeout}
	}
	return &FanficClient{baseURL: baseURL, http: hc, log: log}
}

// BaseURL returns the configured base URL — exported solely for tests.
func (c *FanficClient) BaseURL() string {
	return c.baseURL
}

// FetchDaily calls GET {baseURL}/internal/fanfic/daily.
//
// Return semantics:
//   - (*DailyFanficData, nil) — today's pick, decoded from the envelope.
//   - (nil, nil)              — HTTP 404: no daily fanfic eligible today.
//     This is an expected steady-state outcome, NOT an error — the caller
//     (daily_fanfic spotlight resolver) treats it as an ineligible card.
//   - (nil, err)              — any other non-200 status, a transport
//     failure, or a decode failure.
func (c *FanficClient) FetchDaily(ctx context.Context) (*spotlight.DailyFanficData, error) {
	endpoint := c.baseURL + "/internal/fanfic/daily"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fanfic_client.daily: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warnw("fanfic_client.daily.transport_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("fanfic_client.daily: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No daily fanfic today — expected steady state, not an error.
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		// Read up to 512 bytes for the error message — no full body dump in logs.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if c.log != nil {
			c.log.Warnw("fanfic_client.daily.bad_status", "url", endpoint, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("fanfic_client.daily: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env dailyFanficEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		if c.log != nil {
			c.log.Warnw("fanfic_client.daily.decode_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("fanfic_client.daily: decode: %w", err)
	}
	data := env.Data
	return &data, nil
}
