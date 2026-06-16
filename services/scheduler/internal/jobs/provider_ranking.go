// Package jobs — provider_ranking.go is the Stage 2b daily trigger for the
// Smart Source Selection provider-reliability ranking. The scheduler has no
// ClickHouse connection, so this job POSTs analytics' /internal recompute
// endpoint; analytics runs the aggregate and publishes the player_ranking:*
// Redis keys the catalog serves. Mirrors ReadThresholdJob exactly.
package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

// providerRankingReqTimeout caps the recompute POST. Analytics' handler itself
// bounds the ClickHouse query at 60s; this client timeout is slightly larger so
// the server-side timeout is the one that fires (cleaner error semantics).
const providerRankingReqTimeout = 70 * time.Second

// ProviderRankingJob triggers the analytics daily provider-reliability recompute.
type ProviderRankingJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewProviderRankingJob(cfg *config.JobsConfig, log *logger.Logger) *ProviderRankingJob {
	return &ProviderRankingJob{
		client: &http.Client{Timeout: providerRankingReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs analytics' /internal/player-ranking/recompute. A non-2xx or
// transport error is returned so the JobService metrics wrap records a failure;
// a single missed run is tolerated (the Redis ranking carries a 48h TTL).
func (j *ProviderRankingJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting provider-ranking recompute trigger")
	}
	url := j.config.AnalyticsInternalURL + "/internal/player-ranking/recompute"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build recompute request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post recompute: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("recompute returned status %d", resp.StatusCode)
	}
	if j.log != nil {
		j.log.Info("provider-ranking recompute trigger completed")
	}
	return nil
}
