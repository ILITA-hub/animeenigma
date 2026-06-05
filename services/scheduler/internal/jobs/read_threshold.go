// Package jobs — read_threshold.go is the v4.0 Phase 3 daily trigger that
// closes the D-03 dynamic-read-gate feedback loop. The scheduler has no
// ClickHouse connection (only analytics does), so this job simply POSTs the
// analytics internal recompute endpoint on a daily cron; analytics runs the
// quantile(0.95)(duration_ms) query and publishes the read_thresholds Redis
// hash that every GORM service's ThresholdRefresher snapshots.
//
// AR-EFFECT-01 (dynamic half of D-02/D-03).
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

// readThresholdReqTimeout caps the recompute POST. Analytics' handler itself
// bounds the ClickHouse query at 60s; this client timeout is slightly larger so
// the server-side timeout is the one that fires (cleaner error semantics).
const readThresholdReqTimeout = 70 * time.Second

// ReadThresholdJob triggers the analytics daily db_read P95 recompute. It owns
// no ClickHouse/Redis state — analytics does the compute + publish; this job is
// purely the daily tick over an internal HTTP call (Docker-network only).
type ReadThresholdJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

// NewReadThresholdJob builds the job. The analytics base URL is read from
// config (JobsConfig.AnalyticsInternalURL, default http://analytics:8092).
func NewReadThresholdJob(cfg *config.JobsConfig, log *logger.Logger) *ReadThresholdJob {
	return &ReadThresholdJob{
		client: &http.Client{Timeout: readThresholdReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs analytics' /internal/read-thresholds/recompute. A non-2xx response
// or transport error is returned so the JobService metrics wrap records it as a
// failure — but a single missed run is tolerated (the read_thresholds hash
// carries a 48h expiry on the analytics side, T-03-17).
func (j *ReadThresholdJob) Run(ctx context.Context) error {
	j.log.Info("starting read-threshold recompute trigger")
	url := j.config.AnalyticsInternalURL + "/internal/read-thresholds/recompute"

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
	j.log.Info("read-threshold recompute trigger completed")
	return nil
}
