package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// AnalyticsSink POSTs level transitions to the analytics service's
// Docker-network-only /internal/degradation/transition endpoint, which
// persists them to ClickHouse analytics.degradation_transitions. Best-effort:
// failures are logged and dropped (transitions are rare and the gauges +
// Redis remain authoritative for live behavior; only history is lossy).
type AnalyticsSink struct {
	url        string
	httpClient *http.Client
	log        *logger.Logger
}

// NewAnalyticsSink builds a sink for base URL http://analytics:8092.
func NewAnalyticsSink(baseURL string, log *logger.Logger) *AnalyticsSink {
	return &AnalyticsSink{
		url:        strings.TrimRight(baseURL, "/") + "/internal/degradation/transition",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        log,
	}
}

// Report persists one transition. Never returns an error (see type comment).
func (s *AnalyticsSink) Report(ctx context.Context, t domain.Transition) {
	blob, err := json.Marshal(t)
	if err != nil {
		s.log.Warnw("transition marshal failed", "error", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(blob))
	if err != nil {
		s.log.Warnw("transition request build failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.Warnw("transition report failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		s.log.Warnw("transition report rejected", "status", resp.StatusCode)
	}
}
